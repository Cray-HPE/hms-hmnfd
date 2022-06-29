// MIT License
//
// (C) Copyright [2019-2022] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Cray-HPE/hms-base"
	"github.com/Cray-HPE/hms-hmetcd"
	"github.com/Cray-HPE/hms-msgbus"
)

/////////////////////////////////////////////////////////////////////////////
// Data Structures
/////////////////////////////////////////////////////////////////////////////

// Fanout service URL segment description.

type urlDesc struct {
	url_prefix  string
	url_root    string
	url_version string
	url_port    int
	hostname    string
	fdqn        string
	full_url    string
}

// Application parameters.

type opParams struct {
	Debug           int    `json:"Debug"`
	Help            int    `json:"-"`
	KV_url          string `json:"KV_url"`
	Nosm            int    `json:"Nosm"`
	Port            int    `json:"Port"`
	Scn_in_url      string `json:"Scn_in_url"`
	Scn_max_cache   int    `json:"Scn_max_cache"`
	Scn_cache_delay int    `json:"Scn_cache_delay"`
	Scn_retries     int    `json:"Scn_retries"`
	Scn_backoff     int    `json:"Scn_backoff"`
	SM_retries      int    `json:"SM_retries"`
	SM_timeout      int    `json:"SM_timeout"`
	SM_url          string `json:"SM_url"`
	Telemetry_host  string `json:"Telemetry_host"`
	Use_telemetry   int    `json:"Use_telemetry"`
}

// Transport/client for outbound HTTP stuff

type httpTrans struct {
	transport *http.Transport
	client    *http.Client
}

/////////////////////////////////////////////////////////////////////////////
// Constants and enums
/////////////////////////////////////////////////////////////////////////////

// HB service URL components.  Note that the 'hmnfd' part of the externally
// available URL is taken care of by the service-mesh hmnfd "hostname" if
// operating inside the service mesh, and the API gateway if outside the
// service mesh.  Thus the external URL is:
//
// http://host:port/hmnfd/hmi/v1/...        # generic
// http://cray-hmnfd/hmi/v1/...  # inside svc mesh
// https://api-gateway.default.svc.cluster.local/apis/hmnfd/hmi/v1/... # API GW

const (
	URL_APPNAME       = "hmnfd"
	URL_PREFIX        = "https://"
	URL_BASE          = "hmi"
	URL_V1            = "v1"
	URL_V2            = "v2"
	URL_PORT          = 28600
	URL_SCN           = "scn"
	URL_SUBSCRIBE     = "subscribe"
	URL_SUBSCRIPTIONS = "subscriptions"
	URL_PARAMS        = "params"
	URL_LIVENESS      = "liveness"
	URL_READINESS     = "readiness"
	URL_HEALTH        = "health"
	URL_DELIM         = "/"
	URL_PORT_DELIM    = ":"
)

const (
	KV_URL_BASE     = "mem:"
	SM_URL_BASE     = "https://localhost:27999/hsm/v2"
	SM_SCN_SUB      = "Subscriptions/SCN"
	SM_COMPINFO     = "Component/State"
	SM_STATEDATA    = "State/Components"
	SM_TIMEOUT      = 3
	SM_RETRIES      = 3
	SCN_MAX_CACHE   = 100
	SCN_CACHE_DELAY = 5
	SCN_BACKOFF     = 1
	SCN_RETRIES     = 5
)

const (
	PARAM_START = 1
	PARAM_PATCH = 2
)

const (
	SCN_MSGBUS_TOPIC = "CrayHMSStateChangeNotifications"
)

/////////////////////////////////////////////////////////////////////////////
// Global variables
/////////////////////////////////////////////////////////////////////////////

var serviceName string
var kvHandle hmetcd.Kvi
var hsmsub_chan = make(chan ScnSubscribe, 50000)
var scnWorkPool *base.WorkerPool
var fanoutSyncMode int = 0
var htrans httpTrans
var Running = true
var featureFlag_xnameApiEnable int

var app_params = opParams{
	Debug:           0,
	Help:            0,
	KV_url:          "mem:",
	Nosm:            0,
	Port:            URL_PORT,
	Scn_in_url:      "",
	Scn_max_cache:   SCN_MAX_CACHE,
	Scn_cache_delay: SCN_CACHE_DELAY,
	Scn_backoff:     SCN_BACKOFF,
	Scn_retries:     SCN_RETRIES,
	SM_retries:      SM_RETRIES,
	SM_timeout:      SM_TIMEOUT,
	SM_url:          "https://localhost:27999/hsm/v2",
	Telemetry_host:  "",
	Use_telemetry:   0,
}

var server_url = urlDesc{url_prefix: URL_PREFIX, //https://
	url_root:    URL_BASE, //hmnfd
	url_port:    URL_PORT, //28600
	url_version: URL_V2,   //v2
	hostname:    "",
	fdqn:        "",
	full_url:    "",
}

var msgbusConfig = msgbus.MsgBusConfig{BusTech: msgbus.BusTechKafka,
	Blocking:       msgbus.NonBlocking,
	Direction:      msgbus.BusWriter,
	ConnectRetries: 10,
	Topic:          SCN_MSGBUS_TOPIC,
}
var msgbusHandle msgbus.MsgBusIO = nil

var tbMutex *sync.Mutex = &sync.Mutex{}

/////////////////////////////////////////////////////////////////////////////
// Prints help text on the CLI.
//
// Args, return: None.
/////////////////////////////////////////////////////////////////////////////

func printHelp() {
	fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
	fmt.Printf("  --debug=num             Debug level (Default: 0)\n")
	fmt.Printf("  --kv_url=num            Key-value service base URL. (Default: %s)\n",
		KV_URL_BASE)
	fmt.Printf("  --help                  Help text.\n")
	fmt.Printf("  --nosm                  Don't contact State Manager (for debugging).\n")
	fmt.Printf("  --port=num              HTTPS port to listen on. (Default: %d)\n",
		URL_PORT)
	fmt.Printf("  --scn_backoff=num       Seconds between SCN send retries (Default: %d)\n",
		SCN_BACKOFF)
	fmt.Printf("  --scn_retries=num       Number of times to retry sending SCNs (Default: %d)\n",
		SCN_RETRIES)
	fmt.Printf("  --sm_retries=num        Number of times to retry on State Manager error. (Default: %d)\n",
		SM_RETRIES)
	fmt.Printf("  --sm_timeout=num        Seconds to wait on State Manager accesses. (Default: %d)\n",
		SM_TIMEOUT)
	fmt.Printf("  --sm_url=url            State Manager base URL. (Default: %s)\n",
		SM_URL_BASE)
	fmt.Printf("  --telemetry_host=h:p:t  Hostname:port:topic  of telemetry service\n")
	fmt.Printf("  --use_telemetry         Inject notifications onto telemetry bus (Default: no)\n")
	fmt.Printf("\n")
}

/////////////////////////////////////////////////////////////////////////////
// Generate a JSON string containing the current configurable parameter
// values.
//
// Args: None
// Return: Byte array containing JSON with params; nil on success, error
//         string on error.
/////////////////////////////////////////////////////////////////////////////

func genCurParamJson() ([]byte, error) {
	var ba []byte
	var err error
	ba, err = json.Marshal(app_params)
	return ba, err
}

/////////////////////////////////////////////////////////////////////////////
// Parses the command line and sets operating parameters.
//
// Args, Return: None.
/////////////////////////////////////////////////////////////////////////////

func parseCmdLine() {
	unstr := "xxx"
	unint := -1

	debugP := flag.Int("debug", unint, "Debug level")
	helpP := flag.Bool("help", false, "Help text")
	kv_urlP := flag.String("kv_url", unstr, "Key-Value URL")
	nosmP := flag.Bool("nosm", false, "Don't contact State Manager")
	portP := flag.Int("port", unint, "Port to listen on")
	scn_in_urlP := flag.String("scn_in_url", unstr, "URL where SCNs are received")
	scn_max_cacheP := flag.Int("scn_max_cache", unint, "Max SCNs to cache")
	scn_cache_delayP := flag.Int("scn_cache_delay", unint, "Max time to wait incaching SCNs")
	scn_backoffP := flag.Int("scn_backoff", unint, "Time between SCN send retries")
	scn_retriesP := flag.Int("scn_retries", unint, "Max number of SCN retries")
	sm_retriesP := flag.Int("sm_retries", unint, "Number of times to retry SM on error")
	sm_timeoutP := flag.Int("sm_timeout", unint, "Seconds to wait on SM response")
	sm_urlP := flag.String("sm_url", unstr, "State manager base URL")
	telehostP := flag.String("telemetry_host", unstr, "Telemetry host:port:topic")
	use_teleP := flag.Bool("use_telemetry", false, "Inject notifications onto telemetry bus")

	flag.Parse()

	if *helpP != false {
		printHelp()
		os.Exit(0)
	}

	if *debugP != -1 {
		app_params.Debug = *debugP
		if app_params.Debug < 0 {
			app_params.Debug = 0
		}
	}

	if *kv_urlP != unstr {
		app_params.KV_url = *kv_urlP
	}

	if *nosmP != false {
		app_params.Nosm = 1
	}

	if *portP != unint {
		app_params.Port = *portP
		server_url.url_port = *portP
	}

	if *scn_in_urlP != unstr {
		app_params.Scn_in_url = *scn_in_urlP
	}

	if *scn_max_cacheP != unint {
		app_params.Scn_max_cache = *scn_max_cacheP
	}

	if *scn_cache_delayP != unint {
		app_params.Scn_cache_delay = *scn_cache_delayP
	}

	if *scn_backoffP != unint {
		app_params.Scn_backoff = *scn_backoffP
	}

	if *scn_retriesP != unint {
		app_params.Scn_retries = *scn_retriesP
	}

	if *sm_retriesP != unint {
		app_params.SM_retries = *sm_retriesP
	}

	if *sm_timeoutP != unint {
		app_params.SM_timeout = *sm_timeoutP
	}

	if *sm_urlP != unstr {
		app_params.SM_url = *sm_urlP
	}

	if *telehostP != unstr {
		app_params.Telemetry_host = *telehostP
	}

	if *use_teleP != false {
		app_params.Use_telemetry = 1
	}
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to parse an integer-based environment variable.
//
// envvar(in): Env variable string
// pval(out):  Ptr to an integer to hold the result.
// Return:     None.
/////////////////////////////////////////////////////////////////////////////

func __env_parse_int(envvar string, pval *int) {
	var val string
	if val = os.Getenv(envvar); val != "" {
		ival, err := strconv.ParseUint(val, 0, 64)
		if err != nil {
			log.Printf("ERROR: invalid %s value '%s'.\n", envvar, val)
		} else {
			*pval = int(ival)
		}
	}
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to parse a boolean-based environment variable.
//
// envvar(in): Env variable string
// pval(out):  Ptr to an integer to hold the result.
// Return:     None.
/////////////////////////////////////////////////////////////////////////////

func __env_parse_bool(envvar string, pval *int) {
	var val string
	if val = os.Getenv(envvar); val != "" {
		lcut := strings.ToLower(val)
		if (lcut == "0") || (lcut == "no") || (lcut == "off") || (lcut == "false") {
			*pval = 0
		} else if (lcut == "1") || (lcut == "yes") || (lcut == "on") || (lcut == "true") {
			*pval = 1
		} else {
			log.Printf("ERROR: invalid %s value '%s'.\n", envvar, val)
		}
	}
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to parse a string-based environment variable.
//
// envvar(in): Env variable string
// pval(out):  Ptr to an integer to hold the result.
// Return:     None.
/////////////////////////////////////////////////////////////////////////////

func __env_parse_string(envvar string, pval *string) {
	var val string
	if val = os.Getenv(envvar); val != "" {
		*pval = val
	}
}

/////////////////////////////////////////////////////////////////////////////
// Parse environment variables.  Process any that have the "HMNFD_" prefix
// and set operating parameters with their values.
//
// Args, Return: None.
/////////////////////////////////////////////////////////////////////////////

func parseEnvVars() {
	__env_parse_int("HMNFD_DEBUG", &app_params.Debug)
	__env_parse_string("HMNFD_KV_URL", &app_params.KV_url)
	__env_parse_int("HMNFD_NOSM", &app_params.Nosm)
	__env_parse_int("HMNFD_PORT", &app_params.Port)
	__env_parse_string("HMNFD_SCN_IN_URL", &app_params.Scn_in_url)
	__env_parse_int("HMNFD_SCN_MAX_CACHE", &app_params.Scn_max_cache)
	__env_parse_int("HMNFD_SCN_CACHE_DELAY", &app_params.Scn_cache_delay)
	__env_parse_int("HMNFD_SCN_BACKOFF", &app_params.Scn_backoff)
	__env_parse_int("HMNFD_SCN_RETRIES", &app_params.Scn_retries)
	__env_parse_int("HMNFD_SM_RETRIES", &app_params.SM_retries)
	__env_parse_int("HMNFD_SM_TIMEOUT", &app_params.SM_timeout)
	__env_parse_string("HMNFD_SM_URL", &app_params.SM_url)
	__env_parse_string("HMNFD_TELEMETRY_HOST", &app_params.Telemetry_host)
	__env_parse_int("HMNFD_USE_TELEMETRY", &app_params.Use_telemetry)

	//Feature flags

	__env_parse_int("HMNFD_FEATURE_XNAME_API", &featureFlag_xnameApiEnable)

	//This one is undocumented and used for testing

	__env_parse_int("HMNFD_FANOUT_SYNC", &fanoutSyncMode)
}

/////////////////////////////////////////////////////////////////////////////
// Parse a JSON string containing parameters and assign the parameters based
// on the JSON values.  This is used by the /params API.
//
// param_json(in): JSON string to parse/apply.
// whence(in):     PARAM_START (cmdline), PARAM_PATCH (from ReST API).  The
//                 latter prevents setting of certain parameters, which
//                 can't be changed once the application is running.
// Return:         nil on success, error string on error.
/////////////////////////////////////////////////////////////////////////////

func parseParamJson(param_json []byte, whence int) error {
	var jdata, tpd opParams
	var errstr string

	unint := -1
	unstr := "xxx"
	bad := 0
	tpd = app_params

	//Set default bad values so we know what the unmarshaller actually saw
	jdata.Debug = unint
	jdata.KV_url = unstr
	jdata.Nosm = unint
	jdata.Port = unint
	jdata.Scn_in_url = unstr
	jdata.Scn_max_cache = unint
	jdata.Scn_cache_delay = unint
	jdata.Scn_backoff = unint
	jdata.Scn_retries = unint
	jdata.SM_url = unstr
	jdata.SM_retries = unint
	jdata.SM_timeout = unint
	jdata.Telemetry_host = unstr
	jdata.Use_telemetry = unint

	bberr := json.Unmarshal(param_json, &jdata)
	if bberr != nil {
		var v map[string]interface{}
		var ts string

		//Unmarshal failed, try to find out which field(s) failed.  Note
		//that this method, unmarshalling into an interface, will make a map
		//where the keys are an exact match of the key specified in the
		//request.  This means that if the caller got the value data type
		//wrong, and the case of the key is wrong, we can't detect it.  Thus,
		//we must ToLower the entire JSON string.  Since we don't care about
		//the values, just their types, this means the lower-cased keys will
		//match the map's keys, making it easier to look at all the
		//fields.

		errb := json.Unmarshal([]byte(strings.ToLower(string(param_json))), &v)
		if errb != nil {
			return errb
		}
		mtype := reflect.TypeOf(jdata)
		for i := 0; i < mtype.NumField(); i++ {
			nm := strings.ToLower(mtype.Field(i).Name)
			if v[nm] == nil {
				continue
			}

			ok := true
			switch nm {
			case "debug":
				fallthrough
			case "nosm":
				fallthrough
			case "port":
				fallthrough
			case "scn_max_cache":
				fallthrough
			case "scn_cache_delay":
				fallthrough
			case "use_telemetry":
				_, ok = v[nm].(float64)
				break
			case "kv_url":
				fallthrough
			case "scn_in_url":
				fallthrough
			case "sm_url":
				fallthrough
			case "telemetry_host":
				_, ok = v[nm].(string)
				break
			}
			if !ok {
				ts = fmt.Sprintf("Invalid data type in %s field. ",
					mtype.Field(i).Name)
				log.Printf("%s\n", ts)
				errstr += ts
			}
		}

		rerr := fmt.Errorf(errstr)
		return rerr
	}

	//OK, JSON unmarshal worked.  Now gather up the fields that were found.

	if jdata.Debug != unint {
		tpd.Debug = jdata.Debug
	}
	if jdata.KV_url != unstr {
		tpd.KV_url = jdata.KV_url
	}
	if jdata.Nosm != unint {
		tpd.Nosm = jdata.Nosm
	}
	if jdata.Scn_max_cache != unint {
		tpd.Scn_max_cache = jdata.Scn_max_cache
	}
	if jdata.Scn_cache_delay != unint {
		tpd.Scn_cache_delay = jdata.Scn_cache_delay
	}
	if jdata.Scn_backoff != unint {
		tpd.Scn_backoff = jdata.Scn_backoff
	}
	if jdata.Scn_retries != unint {
		tpd.Scn_retries = jdata.Scn_retries
	}
	if jdata.SM_url != unstr {
		tpd.SM_url = jdata.SM_url
	}
	if jdata.SM_retries != unint {
		tpd.SM_retries = jdata.SM_retries
	}
	if jdata.SM_timeout != unint {
		tpd.SM_timeout = jdata.SM_timeout
	}
	if jdata.Telemetry_host != unstr {
		tpd.Telemetry_host = jdata.Telemetry_host
	}
	if jdata.Use_telemetry != unint {
		tpd.Use_telemetry = jdata.Use_telemetry
	}

	//These are only allowed at startup time

	if jdata.Port != unint {
		if whence == PARAM_PATCH {
			s := fmt.Sprintf("Parameter 'port' can't be changed in PATCH operation; ")
			log.Printf("%s\n", s)
			errstr += s
			bad = 1
		} else {
			tpd.Port = jdata.Port
		}
	}
	if jdata.Scn_in_url != unstr {
		if whence == PARAM_PATCH {
			s := fmt.Sprintf("Parameter 'scn_in_url' can't be changed in PATCH operation; ")
			log.Printf("%s\n", s)
			errstr += s
			bad = 1
		} else {
			tpd.Scn_in_url = jdata.Scn_in_url
		}
	}

	if bad != 0 {
		rerr := fmt.Errorf(errstr)
		return rerr
	}

	app_params = tpd
	if whence == PARAM_START {
		server_url.url_port = app_params.Port
	}
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Thread to connect to telemetry bus.  Retry until successful, or stop
// if we turn telemetry bus usage off before we actuall connect.
//
// Args, return: None
/////////////////////////////////////////////////////////////////////////////

func telebusConnect() {
	for {
		if app_params.Use_telemetry == 0 {
			if msgbusHandle != nil {
				tbMutex.Lock()
				msgbusHandle.Disconnect()
				msgbusHandle = nil
				tbMutex.Unlock()
				log.Printf("Disconnected from telemetry bus.\n")
			}
		} else {
			if msgbusHandle == nil {
				host, port, topic, terr := getTelemetryHost(app_params.Telemetry_host)
				if terr != nil {
					log.Println("ERROR: telemetry host is not set or is invalid:", terr)
				} else {
					if app_params.Debug > 0 {
						log.Printf("Connecting to telemetry host: '%s:%d:%s'\n",
							host, port, topic)
					}
					msgbusConfig.Host = host
					msgbusConfig.Port = port
					msgbusConfig.Topic = topic
					msgbusConfig.ConnectRetries = 1
					tbMutex.Lock()
					msgbusHandle, terr = msgbus.Connect(msgbusConfig)
					if terr != nil {
						log.Println("ERROR connecting to telemetry bus, retrying...:",
							terr)
						msgbusHandle = nil
					} else {
						log.Printf("Connected to Telemetry Bus.\n")
					}
					tbMutex.Unlock()
				}
			}
		}

		time.Sleep(5 * time.Second)
	}
}

/////////////////////////////////////////////////////////////////////////////
// Subscribe to important SCNs from the State Manager, used for subscription
// pruning.
//
// Args, Return: None.
/////////////////////////////////////////////////////////////////////////////

func subscribeMandatoryScn() {
	//TODO: this should be discoverable from hss/base
	var scnStates = []string{base.StateEmpty.String(),
		base.StatePopulated.String(),
		base.StateOff.String(),
		base.StateOn.String(),
		base.StateStandby.String(),
		base.StateHalt.String(),
		base.StateReady.String(),
	}
	var scnRoles = []string{"compute", "service"}
	var enbl = true

	var scn = ScnSubscribe{Subscriber: URL_APPNAME,
		Url:     app_params.Scn_in_url,
		States:  scnStates,
		Enabled: &enbl,
		Roles:   scnRoles,
	}

	log.Printf("Auto-subscribing to SCNs.\n")
	hsmsub_chan <- scn
}

/////////////////////////////////////////////////////////////////////////////
// Print current operating parameters.
//
// Args, Return: None.
/////////////////////////////////////////////////////////////////////////////

func print_app_params() {
	log.Printf("Debug:          %d\n", app_params.Debug)
	log.Printf("Nosm:           %d\n", app_params.Nosm)
	log.Printf("KV_url:         %s\n", app_params.KV_url)
	log.Printf("Port:           %d\n", app_params.Port)
	log.Printf("Scn_in_url:     %s\n", app_params.Scn_in_url)
	log.Printf("Scn_backoff:    %d\n", app_params.Scn_backoff)
	log.Printf("Scn_retries:    %d\n", app_params.Scn_retries)
	log.Printf("SM_retries:     %d\n", app_params.SM_retries)
	log.Printf("SM_timeout:     %d\n", app_params.SM_timeout)
	log.Printf("SM_url:         %s\n", app_params.SM_url)
	log.Printf("Telemetry_host: %s\n", app_params.Telemetry_host)
	log.Printf("Use_telemetry:  %d\n", app_params.Use_telemetry)
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to parse a host:port specification.
//
// hspec(in): Host:port specification.
// Return:    Hostname; Port number; Topic; Error code on failure, or nil.
/////////////////////////////////////////////////////////////////////////////

func getTelemetryHost(hspec string) (string, int, string, error) {
	var err error

	toks := strings.Split(hspec, ":")
	if len(toks) != 3 {
		err = fmt.Errorf("Invalid telemetry host specification '%s', should be host:port:topic format.",
			hspec)
		return "", 0, "", err
	}
	port, perr := strconv.Atoi(toks[1])
	if perr != nil {
		err = fmt.Errorf("Invalid port specification '%s', must be numeric.", toks[1])
		return "", 0, "", err
	}

	return toks[0], port, toks[2], nil
}

/////////////////////////////////////////////////////////////////////////////
// Open up a connection to the KV store and initialize.  Ugh, this is
// complex.  In Dockerfile, you can't create env vars using other env vars.
// And, we get ETCD_HOST and ETCD_PORT from the ETCD operator as env vars.
//
// So, what we'll do is use KV_URL env var in Dockerfile.  If it's not
// empty, use it as is.  If it's empty then we'll check our env vars for
// ETCD_HOST and ETCD_PORT and create a URL from that.  If those aren't
// set, we'll fail.
//
// Args, return: None
/////////////////////////////////////////////////////////////////////////////

func openKV() {
	var kverr error

	if app_params.KV_url == "" {
		eh := os.Getenv("ETCD_HOST")
		ep := os.Getenv("ETCD_PORT")
		if (eh != "") && (ep != "") {
			app_params.KV_url = fmt.Sprintf("http://%s:%s", eh, ep)
			fmt.Printf("INFO: Setting KV URL from ETCD_HOST and ETCD_PORT (%s)\n",
				app_params.KV_url)
		} else {
			//This is a hard fail.  We could just fall back to "mem:" but
			//that will fail in very strange ways in multi-instance mode.
			//Hard fail KV connectivity.

			log.Printf("ERROR: KV URL is not set (no ETCD_HOST/ETCD_PORT and no KV_URL)!  Can't continue.\n")
			for {
				time.Sleep(1000 * time.Second)
			}
		}
	}

	// Try to open connection to ETCD.  This service is worthless until
	// this succeeds, so try forever.  Liveness and readiness probes will
	// fail until it works.

	ix := 1
	for {
		kvHandle, kverr = hmetcd.Open(app_params.KV_url, "")
		if kverr != nil {
			log.Printf("ERROR opening connection to ETCD (attempt %d): %v",
				ix, kverr)
		} else {
			log.Printf("ETCD connection succeeded.\n")
			break
		}
		ix++
		time.Sleep(5 * time.Second)
	}

	//Wait for ETCD connectivity to be OK.  Again, try forever.

	ix = 1
	for {
		kerr := kvHandle.Store("HMNFD_HEALTH_KEY", "HMNFD_OK")
		if kerr == nil {
			log.Printf("K/V health check succeeded.\n")
			break
		}
		log.Printf("ERROR: K/V health key store failed, attempt %d.", ix)
		time.Sleep(5 * time.Second)
		ix++
	}
}

/////////////////////////////////////////////////////////////////////////////
// Entry point.
/////////////////////////////////////////////////////////////////////////////

func main() {
	var err error
	log.Printf("Cray Notification Fanout Service started.\n")

	//Gather ENV vars.  This is the first level of parameters

	parseEnvVars()

	//Parse cmdline params, if any.  These override env vars

	parseCmdLine()

	serviceName, err = base.GetServiceInstanceName()
	if err != nil {
		log.Printf("ERROR: can't get service/host name!  Using 'localhost'.\n")
		serviceName = "localhost"
	}
	log.Printf("Service name: '%s'", serviceName)

	server_url.hostname = serviceName
	server_url.full_url = server_url.url_prefix +
		server_url.hostname +
		URL_PORT_DELIM +
		fmt.Sprintf("%d", server_url.url_port) +
		URL_DELIM + server_url.url_root +
		URL_DELIM + server_url.url_version
	if app_params.Scn_in_url == "" {
		app_params.Scn_in_url = server_url.full_url + URL_DELIM + URL_SCN
	}

	if app_params.Debug > 2 {
		log.Println("server_url: ", server_url.full_url)
	}

	if app_params.Debug > 0 {
		print_app_params()
	}

	// Set up http transport for outbound stuff

	htrans.transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	htrans.client = &http.Client{Transport: htrans.transport,
		Timeout: (time.Duration(app_params.SM_timeout) *
			time.Second),
	}

	// KV store connection

	openKV()

	//Fire up necessary thread funcs

	go telebusConnect()    //telemetry bus connect logic
	go subscribeToHsmScn() //HSM subscriber thread
	go prune()             //subscription prune checker
	go telemetryBusSend()  //service the telemetry bus send requests
	go pruneDeadWood()     //check against component states, prune down nodes
	go handleSCNs()
	go checkSCNCache()

	//Fire up worker pool

	scnWorkPool = base.NewWorkerPool(500, 10000)
	scnWorkPool.Run()

	//Subscribe to SCNs from HSM that are mandatory for operation

	subscribeMandatoryScn()

	log.Printf("Listening on port %d", server_url.url_port)
	log.Printf("URLs:")
	log.Printf("    %s", URL_DELIM+server_url.url_root+
		URL_DELIM+server_url.url_version+
		URL_DELIM+URL_SUBSCRIBE)
	log.Printf("    %s", URL_DELIM+server_url.url_root+
		URL_DELIM+server_url.url_version+
		URL_DELIM+URL_SCN)
	log.Printf("    %s", URL_DELIM+server_url.url_root+
		URL_DELIM+server_url.url_version+
		URL_DELIM+URL_SUBSCRIPTIONS)
	log.Printf("    %s", URL_DELIM+server_url.url_root+
		URL_DELIM+server_url.url_version+
		URL_DELIM+URL_PARAMS)
	log.Printf("    %s", URL_DELIM+server_url.url_root+
		URL_DELIM+server_url.url_version+
		URL_DELIM+URL_LIVENESS)
	log.Printf("    %s", URL_DELIM+server_url.url_root+
		URL_DELIM+server_url.url_version+
		URL_DELIM+URL_READINESS)
	log.Printf("    %s", URL_DELIM+server_url.url_root+
		URL_DELIM+server_url.url_version+
		URL_DELIM+URL_HEALTH)

	routes := generateRoutes()
	router := newRouter(routes)

	port := fmt.Sprintf(":%d", server_url.url_port)
	srv := &http.Server{Addr: port, Handler: router}

	//Set up signal handling for graceful kill

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	idleConnsClosed := make(chan struct{})

	go func() {
		<-c
		Running = false

		//Gracefully shutdown the HTTP server
		lerr := srv.Shutdown(context.Background())
		if lerr != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
		close(idleConnsClosed)
	}()

	log.Printf("Starting up HTTP server.")
	err = srv.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Printf("FATAL: HTTP server ListenandServe failed: %v", err)
	}

	os.Exit(0)
}
