// MIT License
//
// (C) Copyright [2019-2021,2023,2025] Hewlett Packard Enterprise Development LP
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	base "github.com/Cray-HPE/hms-base/v2"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"github.com/gorilla/mux"
)

// A note about subscription tracking and SCN forwarding:
//
// Subscriptions are handled by receiving a JSON payload from a subscriber,
// and taking the fields of that JSON and creating an ETCD key which contains
// the subscriber's XName and all the SCN attributes it subscribes for,
// separated by '_'.  When an SCN comes in, the entire ETCD keyspace is read
// in, and each key is matched to the attributes of the received SCN.
//
// Note that this seems contrary to the hierarchical/directory-like key space
// capabilities of ETCD (which are fake -- ETCD keyspace is really flat, but
// most people emulate it anyway); the reason this is done is that while the
// keys can be made hierarchical, using a flat keyspace is more efficient.  If
// a hierarchical key space is used, then a key would need to be read which
// matches an attribute of the received SCN, and it's value would be the
// list of keys subscribing for that attribute.  This seems like an efficient
// and easy-to-grok key space orgainization, but the result of using that
// method is that the individual subscription records (stored as keys) have
// to then be read one at a time.  On large systems this results in tens of
// thousands of key fetches being done with each SCN; whereas with the flat
// key space, it is only 1 multi-key fetch per SCN.
//
// So it's a matter of convenient key space organization and tons of fetches
// per SCN versus flat key space organization and a single multi-key fetch
// per SCN.  Efficiency is king.

// ALSO NOTE: entities subscribing to SCNs from hmnfd should auto-close the
// connection they use for the subscription request, or else we consume
// a lot of file descriptors on a large system.  The SMS should also set
// ulimit -n to a large number, like 500k, just to cover the cases where
// subscribers don't do this.

/////////////////////////////////////////////////////////////////////////////
// Data Structures
/////////////////////////////////////////////////////////////////////////////

// SCN from HSM; same one is forwarded to subscribers.  This needs to match
// ../sm/sm.go SCNPayload structure format.  TODO: put in a common place?

type Scn struct {
	Components     []string `json:"Components"`
	Enabled        *bool    `json:"Enabled,omitempty"`
	Flag           string   `json:"Flag,omitempty"`
	Role           string   `json:"Role,omitempty"`
	SubRole        string   `json:"SubRole,omitempty"`
	SoftwareStatus string   `json:"SoftwareStatus,omitempty"`
	State          string   `json:"State,omitempty"`
	Timestamp      string   `json:"Timestamp,omitempty"`
}

// SCN subscription.  Used for hmnfd->HSM subscriptions and also node->hmnfd
// subscriptions (/subscribe API).  For hmnfd, one can subscribe for > 1 type
// of SCN if desired, which will go to the same URL.  For HSM, we may keep
// them all separate.  NOTE: when using it for HSM, don't use Components,
// as it will be ignored.  This needs to match ../sm/sm.go SCNPostSubscription
// structure format.  TODO: put in a common place?

type ScnSubscribe struct {
	Components          []string `json:"Components,omitempty"`          //SCN components (usually nodes)
	Subscriber          string   `json:"Subscriber,omitempty"`          //[service@]xname (nodes) or 'hmnfd'
	SubscriberComponent string   `json:"SubscriberComponent,omitempty"` //xname (nodes) or 'hmnfd'
	SubscriberAgent     string   `json:"SubscriberAgent,omitempty"`     //agent
	Enabled             *bool    `json:"Enabled,omitempty"`             //true==all enable/disable SCNs
	Roles               []string `json:"Roles,omitempty"`               //Subscribe to role changes
	SubRoles            []string `json:"SubRoles,omitempty"`            //Subscribe to sub-role changes
	SoftwareStatus      []string `json:"SoftwareStatus,omitempty"`      //Subscribe to these SW SCNs
	States              []string `json:"States,omitempty"`              //Subscribe to these HW SCNs
	Url                 string   `json:"Url"`                           //URL to send SCNs to
}

// JSON data for subscription deletion coming into /subscribe
// NOTE: this is v1 only (deprecated)

type NodeSubscriptionDelete struct {
	Subscriber string `json:"Subscriber"`
	Url        string `json:"Url"`
}

// Data stored in ETCD subscription records

type SubData struct {
	Url      string   `json:"Url"`
	ScnNodes []string `json:"ScnNodes"`
}

// Subscription list returned by /subscriptions

type SubscriptionList struct {
	SubscriptionList []ScnSubscribe `json:"SubscriptionList"`
}

// REST API routing

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route

/////////////////////////////////////////////////////////////////////////////
// Constants
/////////////////////////////////////////////////////////////////////////////

// Wild cards for subscriptions.  TODO: there may be more of these.
const (
	WC_ALLNODES = "allnodes"
	WC_ALL      = "all"
)

//Subscriber ETCD key stuff.  Records appear as follows:
//  sub#xname[#hs.state[.state...]][#sws.swstate[.swstate...]][#enbl.enbl][#roles.Role[.Role...]][#svc.Svc]
//
// There can be any number of 'state', 'swstate', 'roles', and 'subroles'
// subtypes.

const (
	HSM_SUBS_KEY              = "hsmsubs"
	SUBSCRIBER_KEY_PREFIX     = "sub"
	SUBSCRIBER_SVC_DELIM      = "@"
	SUBSCRIBER_KEY_DELIM      = "#"
	SUBSCRIBER_KEYCAT_DELIM   = "."
	SUBSCRIBER_KEY_HWS        = "hs"
	SUBSCRIBER_KEY_SWS        = "ss"
	SUBSCRIBER_KEY_SVC        = "svc"
	SUBSCRIBER_KEY_ROLES      = "roles"
	SUBSCRIBER_KEY_SUBROLES   = "subroles"
	SUBSCRIBER_KEY_ENBL       = "enbl"
	SUBSCRIBER_KEYRANGE_START = "sub#a"
	SUBSCRIBER_KEYRANGE_END   = "sub#z"
	SUBSCRIBER_TARG_START     = "x"
	SUBSCRIBER_TARG_END       = "z"

	SUBSCRIBER_TOKNUM_XNAME  = 1
	SUBSCRIBER_TOKNUM_STYPES = 2

	SERVICE_TOKNUM_SVC   = 0
	SERVICE_TOKNUM_XNAME = 1

	KV_PARAM_KEY = "hmnfd_params"
)

/////////////////////////////////////////////////////////////////////////////
// Global Variables
/////////////////////////////////////////////////////////////////////////////

// Local map of components to prune from SCN subscriber list

var prunemap = make(map[string]bool)
var prunemap_mutex = &sync.Mutex{}

var kq_chan = make(chan string, 10000)
var scnQ = make(chan Scn, 10000)

var jdCache Scn
var jdCount = 0
var jdcMutex = &sync.Mutex{}

/////////////////////////////////////////////////////////////////////////////
// Find the intersection of 2 string arrays.  The arrays don't have
// to be the same length.  The compares are case sensitive.  Note that
// 'subarr' can have wildcards in it like "all", "allnodes", etc.
// NOTE: the passed-in string arrays are assumed to have been lower-cased.
//
// subarr(in): Subscription target array.
// hsmarr(in): HSM's SCN target array
// Return:    String array containing only elements found in both arrays.
/////////////////////////////////////////////////////////////////////////////

func intersect(subarr []string, hsmarr []string) []string {
	tlen := int(math.Max(float64(len(subarr)), float64(len(hsmarr))))
	dmap := make(map[string]bool, tlen)
	osa := make([]string, tlen)

	ix := 0

	//The rule is (for now) that if wildcards are used, they must be
	//the only thing in the target list, and only one is allowed.
	//TODO: at some point we may need to handle > 1 of these.

	lcc := subarr[0]
	if lcc == WC_ALL {
		//copy hsmarr into return value
		for iy := 0; iy < len(hsmarr); iy++ {
			osa[ix] = hsmarr[iy]
			ix++
		}
	} else if lcc == WC_ALLNODES {
		//copy only SCN targets which are nodes
		var regex *regexp.Regexp
		regex = regexp.MustCompile("^x.*b[0-9]+n[0-9]+$")
		for iy := 0; iy < len(hsmarr); iy++ {
			if regex.MatchString(hsmarr[iy]) {
				osa[ix] = hsmarr[iy]
				ix++
			}
		}
	} else {
		for ix := 0; ix < len(subarr); ix++ {
			dmap[subarr[ix]] = true
		}

		for _, v := range hsmarr {
			_, ok := dmap[v]
			if ok {
				osa[ix] = v
				ix++
				delete(dmap, v) //assures only one match
			}
		}
	}

	return osa[:ix]
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to determine if a given state equates to
// "unavailable".
//
// state(in):  State to examine
// Return:     true if state == unavailable, else false.
/////////////////////////////////////////////////////////////////////////////

func isStateUnavailable(scn Scn) bool {
	switch strings.ToLower(scn.State) {
	case strings.ToLower(base.StateEmpty.String()):
		fallthrough
	case strings.ToLower(base.StateOff.String()):
		fallthrough
	case strings.ToLower(base.StateHalt.String()):
		return true
	}
	if (scn.Enabled != nil) && (*scn.Enabled == false) {
		return true
	}
	//TODO: need checking for SW status -- dunno which ones are bad!

	return false
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to ToLower() all component strings of an Scn struct.
// This saves time over doing ToLower of the request body and unmarshalling
// again.  Having everything in lower case saves headaches for things like
// KV keys which need to always be consistent.
//
// sp(inout):  Ptr to an Scn data object
// Return:     None
/////////////////////////////////////////////////////////////////////////////

func scnToLower(sp *Scn) {
	for ix := 0; ix < len(sp.Components); ix++ {
		sp.Components[ix] = strings.ToLower(sp.Components[ix])
	}
	sp.Flag = strings.ToLower(sp.Flag)
	sp.Role = strings.ToLower(sp.Role)
	sp.SubRole = strings.ToLower(sp.SubRole)
	sp.SoftwareStatus = strings.ToLower(sp.SoftwareStatus)
	sp.State = strings.ToLower(sp.State)
}

/////////////////////////////////////////////////////////////////////////////
// Perform a prune operation.  Given a map of components to prune, delete
// the subscription records from ETCD.
//
// Args, Return: None.
/////////////////////////////////////////////////////////////////////////////

func subPrune() {
	kvlist, kverr := kvHandle.GetRange(SUBSCRIBER_KEYRANGE_START,
		SUBSCRIBER_KEYRANGE_END)

	if kverr != nil {
		log.Println("ERROR retrieving SCN key list:", kverr)
		return
	}

	for _, sub := range kvlist {
		//Tokenize the key and separate out the relevant bits, and match
		//subscription keys.
		//Prune requests from DELETE operations require an exact match of the
		//'service' in the subscription.  Others will just be an XNAME match
		//and will happen during pruning based on an SCN that takes nodes
		//into bad states.

		toks := strings.Split(sub.Key, SUBSCRIBER_KEY_DELIM)
		xname := toks[SUBSCRIBER_TOKNUM_XNAME]
		subscriber := "bad_sub"

		ix := strings.Index(sub.Key, SUBSCRIBER_KEY_SVC)
		if ix > 0 {
			tt := strings.Split(sub.Key[(ix+len(SUBSCRIBER_KEY_SVC)+1):], SUBSCRIBER_KEY_DELIM)
			subscriber = tt[0] + SUBSCRIBER_SVC_DELIM + toks[SUBSCRIBER_TOKNUM_XNAME]
		}

		val, ok := prunemap[xname]
		val2, ok2 := prunemap[subscriber]
		if (ok && val) || (ok2 && val2) {
			//prune
			if app_params.Debug > 1 {
				log.Printf("PRUNING: '%s'\n", sub.Key)
			}
			err := kvHandle.Delete(sub.Key)
			if err != nil {
				log.Println("WARNING, key not deleted:", sub.Key, ":", err)
				//play it safe and don't delete the prunemap entry.  If the node
				//is really dead, and we just can't find the subscription, it
				//will get deleted eventually by 400 failures.
			}
		}
	}
}

/////////////////////////////////////////////////////////////////////////////
// Timer-based pruning aggregation.  This function prunes subscriptions from
// ETCD when they are no longer needed (SCN contains nodes with subscriptions,
// or a subscriber stops responding to SCN POSTs).  There is also a locally
// cached "prune map" which allows prevention of sending SCNs to subscribers
// to be pruned; this bridges the gap from when a subscriber needs to be
// pruned until it is actually pruned in ETCD.
//
// Args,Return: None.
/////////////////////////////////////////////////////////////////////////////

func prune() {
	for {
		time.Sleep(10 * time.Second)
		if len(prunemap) > 0 {
			//Perform the prune
			prunemap_mutex.Lock()
			subPrune()
			for pm := range prunemap {
				delete(prunemap, pm)
			}
			prunemap_mutex.Unlock()
		}
	}
}

/////////////////////////////////////////////////////////////////////////////
// Send SCN to the telemetry bus.  This is a thread -- prevents blocking
// when injecting onto the bus, which can happen if Kafka is restarting,
// doing leader election etc., misc. transient errors.
/////////////////////////////////////////////////////////////////////////////

func telemetryBusSend() {
	var retry int
	max_tries := 5

	for {
		select {
		case kmsg := <-kq_chan:
			for retry = 0; retry < max_tries; retry++ {
				if app_params.Use_telemetry != 0 {
					if app_params.Debug > 2 {
						log.Printf("Sending SCN to telemetry bus [try %d]: '%s'\n",
							(retry + 1), kmsg)
					}
					bad := false
					tbMutex.Lock()
					if msgbusHandle != nil {
						err := msgbusHandle.MessageWrite(kmsg)
						if err != nil {
							log.Println("ERROR injecting telemetry data:", err)
							bad = true
						}
					}
					tbMutex.Unlock()
					if !bad {
						break
					}
				}
				time.Sleep(time.Second)
			}
			if retry >= max_tries {
				log.Printf("Telemetry inject failed, retries exhausted.\n")
			}
		}
	}
}

/////////////////////////////////////////////////////////////////////////////
// Inject an SCN onto the telemetry/kafka bus.  This function will never
// block; it will place the SCN onto a queue which a thread func will
// process.  If the Q becomes full, it's due to kafka being in a bad state,
// in which case we'll lose the SCN injection.
//
// tmsg(in):  SCN data to send
/////////////////////////////////////////////////////////////////////////////

func sendToTelemetryBus(tmsg Scn) {
	if app_params.Use_telemetry != 0 {
		ba, baerr := json.Marshal(&tmsg)
		if baerr != nil {
			log.Printf("ERROR: can't marshal SCN for telemetry: %v", baerr)
			return
		}
		select {
		case kq_chan <- string(ba):
		default:
			log.Printf("ERROR: Telemetry queue is full, cannot inject...\n")
		}
	}
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function which handles the details of a subscribe POST/PATCH
// operation errors.
//
// errinst(in): URL handling the request (for error reporting).
// w(in):       HTTP response writer.
// body(in);    HTTP request body.
// Return:      None.
/////////////////////////////////////////////////////////////////////////////

func handleSubscribePostError(errinst string, w http.ResponseWriter, body []byte) {
	var v map[string]interface{}
	var errstr string
	var jdraw ScnSubscribe

	errb := json.Unmarshal([]byte(strings.ToLower(string(body))), &v)
	if errb != nil {
		log.Println("Error on generic unmarshal:", errb)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Error unmarshalling JSON payload",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}
	mtype := reflect.TypeOf(jdraw)
	for i := 0; i < mtype.NumField(); i++ {
		nm := strings.ToLower(mtype.Field(i).Name)
		if v[nm] == nil {
			continue
		}

		ok := true
		switch nm {
		case "subscriber":
			fallthrough
		case "url":
			_, ok = v[nm].(string)
			if !ok {
				errstr += fmt.Sprintf("Invalid data type in %s field. ",
					mtype.Field(i).Name)
			}
			break
		case "components":
			fallthrough
		case "states":
			fallthrough
		case "softwarestatus":
			fallthrough
		case "roles":
		case "subroles":
			//If any elements are not strings, this check will fail.
			//No need to iterate through them, unless we'd want to
			//pinpoint the exact one(s) that are wrong.  Not worth it.
			_, ok = v[nm].([]string)
			if !ok {
				errstr += fmt.Sprintf("Invalid data type in %s array field. ",
					mtype.Field(i).Name)
			}
			break
		case "enabled":
			_, ok = v[nm].(bool)
			if !ok {
				errstr += fmt.Sprintf("Invalid data type in %s field. ",
					mtype.Field(i).Name)
			}
			break
		}
	}

	log.Printf("Error unmarshaling JSON: %s\n", errstr)
	pdet := base.NewProblemDetails("about:blank",
		"Invalid Request",
		errstr,
		errinst, http.StatusBadRequest)
	base.SendProblemDetails(w, pdet, 0)
	return
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function which handles the details of a subscribe DELETE
// operation error.
//
// errinst(in): URL handling the request (for error reporting).
// w(in):       HTTP response writer.
// body(in);    HTTP request body.
// Return:      None.
/////////////////////////////////////////////////////////////////////////////

func handleSubscribeDeleteError(errinst string, w http.ResponseWriter, body []byte) {
	var v map[string]interface{}
	var errstr string
	var jdata NodeSubscriptionDelete

	errb := json.Unmarshal([]byte(strings.ToLower(string(body))), &v)
	if errb != nil {
		log.Println("Error on generic unmarshal:", errb)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Error unmarshalling JSON payload",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}
	mtype := reflect.TypeOf(jdata)
	for i := 0; i < mtype.NumField(); i++ {
		nm := strings.ToLower(mtype.Field(i).Name)
		if v[nm] == nil {
			continue
		}

		ok := true
		switch nm {
		case "subscriber":
			fallthrough
		case "url":
			_, ok = v[nm].(string)
			if !ok {
				errstr += fmt.Sprintf("Invalid data type in %s field. ",
					mtype.Field(i).Name)
			}
			break
		}
	}

	log.Printf("Error unmarshaling JSON: %s", errstr)
	pdet := base.NewProblemDetails("about:blank",
		"Invalid Request",
		errstr,
		errinst, http.StatusBadRequest)
	base.SendProblemDetails(w, pdet, 0)
	return
}

/////////////////////////////////////////////////////////////////////////////
// Checks a subscribe POST/PATCH operation to be sure the fields are correct
// and the mandatory ones are present.
//
// jdraw(in): Subscription structure from JSON unmarshal.
// Return:    nil on success, error string on error.
/////////////////////////////////////////////////////////////////////////////

func checkSubscription_v1(jdraw ScnSubscribe) error {
	if jdraw.Subscriber == "" {
		return fmt.Errorf("Subscription request missing Subscriber field.")
	} else {
		toks := strings.Split(jdraw.Subscriber, "@")
		if len(toks) > 2 {
			return fmt.Errorf("Subscription request Subscriber field has invalid format.")
		}
	}
	if len(jdraw.Components) == 0 {
		return fmt.Errorf("Subscription request missing Components array field.")
	}
	if jdraw.Url == "" {
		return fmt.Errorf("Subscription request missing Url field.")
	}

	//At least one of the following must be present

	if (len(jdraw.States) == 0) && (len(jdraw.SoftwareStatus) == 0) &&
		(jdraw.Enabled == nil) && (len(jdraw.Roles) == 0) &&
		(len(jdraw.SubRoles) == 0) {
		return fmt.Errorf("Subscription request needs at least one of: States, SoftwareStatus, Roles, SubRoles.")
	}
	return nil
}

func checkSubscription_v2(jdraw ScnSubscribe) error {
	if len(jdraw.Components) == 0 {
		return fmt.Errorf("Subscription request missing Components array field.")
	}
	if jdraw.Url == "" {
		return fmt.Errorf("Subscription request missing Url field.")
	}

	//At least one of the following must be present

	if (len(jdraw.States) == 0) && (len(jdraw.SoftwareStatus) == 0) &&
		(jdraw.Enabled == nil) && (len(jdraw.Roles) == 0) &&
		(len(jdraw.SubRoles) == 0) {
		return fmt.Errorf("Subscription request needs at least one of: States, SoftwareStatus, Roles, SubRoles.")
	}
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Does the dirty work of a POST operation on the /subscribe API endpoint.
//
// w(in):  HTTP response writer.
// r(in):  HTTP request.
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func doSubscribePost(w http.ResponseWriter, r *http.Request) {
	var jdata ScnSubscribe
	var subscriber_xname string

	errinst := "/" + URL_SUBSCRIBE
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error on message read:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Error reading inbound request body",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	err = json.Unmarshal([]byte(strings.ToLower(string(body))), &jdata)
	if err != nil {
		handleSubscribePostError(errinst, w, body)
		return
	}

	//Make sure all mandatory fields are present.

	err = checkSubscription_v1(jdata)
	if err != nil {
		log.Println("Missing subscription payload fields:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			err.Error(),
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	if app_params.Debug > 0 {
		log.Printf("Received a subscription POST request, payload '%s'.\n",
			string(body))
	}

	//Subscriber can be a plain XName, or 'service@XName'.

	subscriber_xname = jdata.Subscriber

	ix := strings.Index(jdata.Subscriber, SUBSCRIBER_SVC_DELIM)
	if ix != -1 {
		subscriber_xname = jdata.Subscriber[ix+1:]
	}

	if xnametypes.GetHMSType(subscriber_xname) == xnametypes.HMSTypeInvalid {
		//This is not a valid XName.  We can't accept this, since it will
		//make pruning not work.
		log.Printf("Invalid subscriber XName: '%s'.\n", subscriber_xname)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Subcriber field is not a valid XName",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	//Construct the subscription ETCD key and see if it already exists.

	subKey := makeSubscriptionKey_V1(jdata)

	//Check if the key exists.  If so, that is an error.  TODO: should
	//probably also check all subscriptions for this XName, looking at
	//subscribers and URL to be sure we don't duplicate that way.
	//That's a little expensive, and the chances of it happening are small.

	_, sok, serr := kvHandle.Get(subKey)
	if serr != nil {
		log.Println("ERROR fetching subscription key:", subKey, ":", serr)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Failed KV service GET operation",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	if sok {
		//Key exists.  This is an error for a POST
		log.Printf("ERROR, found existing subscription: '%s', not allowed in POST.\n",
			subKey)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Subscription exists, cannot modify in POST operation",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	//No existing key.  Make one.

	err = makeSubscriptionEntry(jdata.Components, jdata.Url, subKey)
	if err != nil {
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			err.Error(),
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}
	hsmsub_chan <- jdata //subscribe to SCN from HSM

	w.Header().Add("Connection", "close")
	w.WriteHeader(http.StatusOK)
}

/////////////////////////////////////////////////////////////////////////////
// Does the dirty work of a PATCH operation on the /subscribe API endpoint.
//
// w(in):  HTTP response writer.
// r(in):  HTTP request.
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func doSubscribePatch(w http.ResponseWriter, r *http.Request) {
	var jdata ScnSubscribe

	errinst := "/" + URL_SUBSCRIBE
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error on message read:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Error reading inbound request body",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	err = json.Unmarshal([]byte(strings.ToLower(string(body))), &jdata)
	if err != nil {
		handleSubscribePostError(errinst, w, body)
		return
	}

	//Make sure all fields are present.  All are mandatory.

	err = checkSubscription_v1(jdata)
	if err != nil {
		log.Println("Missing subscription payload fields:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			err.Error(),
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	if app_params.Debug > 0 {
		log.Printf("Received a subscription PATCH request, payload: '%s'\n",
			string(body))
	}

	subscriber_xname := jdata.Subscriber
	ix := strings.Index(jdata.Subscriber, SUBSCRIBER_SVC_DELIM)
	if ix > 0 {
		subscriber_xname = jdata.Subscriber[ix+1:]
	}

	startKey := SUBSCRIBER_KEY_PREFIX + SUBSCRIBER_KEY_DELIM + subscriber_xname
	endKey := startKey + SUBSCRIBER_TARG_END

	skvlist, serr := kvHandle.GetRange(startKey, endKey)
	if serr != nil {
		log.Println("ERROR fetching subscription keys:", serr)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Key/Value ETCD service GET operation failed",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	svcKeyDesc := SUBSCRIBER_KEY_SVC + SUBSCRIBER_KEYCAT_DELIM

	for _, kv := range skvlist {
		toks := strings.Split(kv.Key, SUBSCRIBER_KEY_DELIM)
		svcKey := toks[SUBSCRIBER_TOKNUM_XNAME]

		//Find the service token, if any, match with jdata.Subscriber

		ix := strings.Index(kv.Key, svcKeyDesc)
		if ix > 0 {
			iy := strings.Index(kv.Key[ix:], SUBSCRIBER_KEY_DELIM)
			if iy == -1 {
				svcKey = kv.Key[ix+len(svcKeyDesc):] + SUBSCRIBER_SVC_DELIM + svcKey
			} else {
				svcKey = kv.Key[ix+len(svcKeyDesc):iy-1] + SUBSCRIBER_SVC_DELIM + svcKey
			}
		}

		//Read the key's value and match the Url

		var sd SubData
		err := json.Unmarshal([]byte(kv.Value), &sd)
		if err != nil {
			log.Printf("ERROR unmarshaling ETCD key '%s' data: ", kv.Key)
			log.Println(err)
			pdet := base.NewProblemDetails("about:blank",
				"Internal Server Error",
				"Error unmarshalling ETCD KV value",
				errinst, http.StatusInternalServerError)
			base.SendProblemDetails(w, pdet, 0)
			return
		}

		if (sd.Url == jdata.Url) && (svcKey == jdata.Subscriber) {
			//Match!  See if this subscription is an exact match. If so,
			//just replace the contents (key val).  If not, delete this key
			//and make a new one.

			var newSD SubData
			newSD.Url = jdata.Url
			newSD.ScnNodes = jdata.Components

			exKey := makeSubscriptionKey_V1(jdata)
			if exKey != kv.Key {
				if app_params.Debug > 1 {
					log.Printf("Replacing subscription key '%s' with '%s'.\n",
						kv.Key, exKey)
				}
				err := kvHandle.Delete(kv.Key)
				if err != nil {
					log.Println("ERROR deleting key:", kv.Key, ":", err)
					pdet := base.NewProblemDetails("about:blank",
						"Internal Server Error",
						"Error deleting ETCD KV value",
						errinst, http.StatusInternalServerError)
					base.SendProblemDetails(w, pdet, 0)
					return
				}
			} else {
				if app_params.Debug > 1 {
					log.Printf("Replacing subscription key '%s' value only.\n",
						kv.Key)
				}
			}
			makeSubscriptionEntry(jdata.Components, jdata.Url, exKey)
			break
		}
	}

	w.WriteHeader(http.StatusOK)
}

/////////////////////////////////////////////////////////////////////////////
// Does the dirty work of a DELETE operation on the /subscribe API endpoint.
//
// TODO: this is expensive -- we have to get all keys each time there is a
// subscription delete.  We may have to batch these up and do them in chunks.
// We can use the prune map to bridge the gap between the delete request
// and the actual ETCD key removal.
//
// w(in):  HTTP response writer.
// r(in):  HTTP request.
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func doSubscribeDelete(w http.ResponseWriter, r *http.Request) {
	var jdata NodeSubscriptionDelete
	errinst := "/" + URL_SUBSCRIBE

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error on message read:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Error reading inbound request",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
	}
	err = json.Unmarshal(body, &jdata)
	if err != nil {
		handleSubscribeDeleteError(errinst, w, body)
		return
	}

	//Verify that all fields are populated.

	if jdata.Subscriber == "" {
		log.Printf("ERROR, missing 'Subscriber' field in subscription request.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Missing Subscriber field in request",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
	}

	if jdata.Url == "" {
		log.Printf("ERROR, missing 'Url' field in subscription request.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Missing Url field in request",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
	}

	log.Printf("Received a subscription DELETE request.\n")

	skvlist, serr := kvHandle.GetRange(SUBSCRIBER_KEYRANGE_START, SUBSCRIBER_KEYRANGE_END)
	if serr != nil {
		log.Println("ERROR fetching subscription keys:", serr)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Key/Value ETCD service GET operation failed",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	for _, sub := range skvlist {
		//Get the service name from the key.  It can be a simple XName or svc@XName.
		//Gotta get the XName token (always first after sub), then get optional
		//service name (defined by svc.xxx in the key)

		toks := strings.Split(sub.Key, SUBSCRIBER_KEY_DELIM)
		subsvc := toks[SUBSCRIBER_TOKNUM_XNAME] //first assume a plain XName
		ix := strings.Index(sub.Key, SUBSCRIBER_KEY_SVC)
		if ix > 0 {
			tt := strings.Split(sub.Key[(ix+len(SUBSCRIBER_KEY_SVC)+1):], SUBSCRIBER_KEY_DELIM)
			subsvc = tt[0] + SUBSCRIBER_SVC_DELIM + toks[SUBSCRIBER_TOKNUM_XNAME]
		}
		if app_params.Debug > 1 {
			log.Printf("Found subscriber key: '%s'\n", subsvc)
		}
		if strings.ToLower(subsvc) == strings.ToLower(jdata.Subscriber) {
			if app_params.Debug > 1 {
				log.Printf("MATCHED subscription key for deletion: '%s'\n",
					jdata.Subscriber)
			}
			//Unmarshal the key value, get the URL.
			var sd SubData
			err := json.Unmarshal([]byte(sub.Value), &sd)
			if err != nil {
				log.Printf("ERROR unmarshaling ETCD key '%s' data: ", sub.Key)
				log.Println(err)
				pdet := base.NewProblemDetails("about:blank",
					"Internal Server Error",
					"Error unmarshalling ETCD KV value",
					errinst, http.StatusInternalServerError)
				base.SendProblemDetails(w, pdet, 0)
				return
			}
			if strings.ToLower(jdata.Url) == strings.ToLower(sd.Url) {
				err := kvHandle.Delete(sub.Key)
				if err != nil {
					log.Println("WARNING, key not deleted:", sub.Key, ":", err)
				} else {
					//Put this in the pruning map to prevent stuff in the Q
					//destined for this node from getting sent.
					prunemap_mutex.Lock()
					prunemap[subsvc] = true
					prunemap_mutex.Unlock()
				}
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

/////////////////////////////////////////////////////////////////////////////
// Convenience func to create a "raw" list of states, SW statuses, enabled,
// roles, and subroles, used for SCN matching.
//
// jdata(in): SCN from HSM.
// Return:    String containing "raw" SCN attributes.
/////////////////////////////////////////////////////////////////////////////

func getSCNAttrs(jdata Scn) []string {
	var scnAttrs []string

	if jdata.Enabled != nil {
		scnAttrs = append(scnAttrs, SUBSCRIBER_KEY_ENBL)
	}
	if jdata.Role != "" {
		scnAttrs = append(scnAttrs, strings.ToLower(jdata.Role))
	}
	if jdata.SubRole != "" {
		scnAttrs = append(scnAttrs, strings.ToLower(jdata.SubRole))
	}
	if jdata.State != "" {
		scnAttrs = append(scnAttrs, strings.ToLower(jdata.State))
	}
	if jdata.SoftwareStatus != "" {
		scnAttrs = append(scnAttrs, strings.ToLower(jdata.SoftwareStatus))
	}
	return scnAttrs
}

/////////////////////////////////////////////////////////////////////////////
// Create a subscription ETCD key based on a subscription request.
//
// Format is: sub#xname[#hs.state[.state...]][#sws.swstate[.swstate...]][#enbl.enbl][#roles.Role[.Role...]][#subroles.SubRole[.SubRole...]][#svc.Svc]
//
// jdata(in):      Subscription request data
// subXName(in):   Subscriber component name, or "hmnfd"
// subSvcname(in): Subscriber SW agent name, blank if xname == "hmnfd"
// Return:         ETCD key used for storing subscription.
/////////////////////////////////////////////////////////////////////////////

func makeSubscriptionKey_V2(jdata ScnSubscribe, subXName, subSvcName string) string {
	var subkey string

	//Start with the 'base'

	subkey = SUBSCRIBER_KEY_PREFIX + SUBSCRIBER_KEY_DELIM + subXName

	//States

	if len(jdata.States) > 0 {
		subkey = subkey + SUBSCRIBER_KEY_DELIM + SUBSCRIBER_KEY_HWS
		for _, state := range jdata.States {
			subkey = subkey + SUBSCRIBER_KEYCAT_DELIM + strings.ToLower(state)
		}
	}

	//SW states

	if len(jdata.SoftwareStatus) > 0 {
		subkey = subkey + SUBSCRIBER_KEY_DELIM + SUBSCRIBER_KEY_SWS
		for _, swstate := range jdata.SoftwareStatus {
			subkey = subkey + SUBSCRIBER_KEYCAT_DELIM + strings.ToLower(swstate)
		}
	}

	//Enabled

	if jdata.Enabled != nil {
		subkey = subkey + SUBSCRIBER_KEY_DELIM + SUBSCRIBER_KEY_ENBL +
			SUBSCRIBER_KEYCAT_DELIM + SUBSCRIBER_KEY_ENBL
	}

	//Roles

	if len(jdata.Roles) > 0 {
		subkey = subkey + SUBSCRIBER_KEY_DELIM + SUBSCRIBER_KEY_ROLES
		for _, role := range jdata.Roles {
			subkey = subkey + SUBSCRIBER_KEYCAT_DELIM + strings.ToLower(role)
		}
	}

	//SubRoles

	if len(jdata.SubRoles) > 0 {
		subkey = subkey + SUBSCRIBER_KEY_DELIM + SUBSCRIBER_KEY_SUBROLES
		for _, subrole := range jdata.SubRoles {
			subkey = subkey + SUBSCRIBER_KEYCAT_DELIM + strings.ToLower(subrole)
		}
	}

	//Service, if any

	if subSvcName != "" {
		subkey = subkey + SUBSCRIBER_KEY_DELIM + SUBSCRIBER_KEY_SVC +
			SUBSCRIBER_KEYCAT_DELIM + subSvcName
	}

	return subkey
}

func makeSubscriptionKey_V1(jdata ScnSubscribe) string {
	toks := strings.Split(jdata.Subscriber, SUBSCRIBER_SVC_DELIM)
	xname := strings.ToLower(jdata.Subscriber)
	svcname := ""

	if len(toks) > 1 {
		xname = strings.ToLower(toks[SERVICE_TOKNUM_XNAME])
		svcname = strings.ToLower(toks[SERVICE_TOKNUM_SVC])
	}

	return makeSubscriptionKey_V2(jdata, xname, svcname)
}

/////////////////////////////////////////////////////////////////////////////
// Handle an SCN coming from StateMgr.  Run the list of subscriptions and put an
// SCN into the delivery Q.
//
// w(in):  HTTP response writer
// r(in):  HTTP request
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func scnHandler(w http.ResponseWriter, r *http.Request) {
	var jdata Scn

	errinst := "/" + URL_SCN

	if r.Method != "POST" {
		log.Printf("ERROR: request is not a POST.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Only POST operations supported",
			errinst, http.StatusMethodNotAllowed)
		//It is required to have an "Allow:" header with this error
		w.Header().Add("Allow", "POST")
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	body, err := ioutil.ReadAll(r.Body)

	//Need 2 copies of the unmarshalled data, on in all lower-case (saves
	//lots of ToLower() calls), and once with the case left intact (for passing
	//data on to subscribers).  We'll unmarshal only once and then ToLower()
	//the members.  This is more manual than just unmarshalling twice, once
	//with the normal input and once with ToLower()'d input, but unmarshalling
	//is expensive.  Manually ToLowering the unmarshalled data is twice as
	//fast as unmarshalling twice.

	err = json.Unmarshal(body, &jdata)
	if err != nil {
		log.Println("Error unmarshaling JSON:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Error unmarshalling SCN JSON",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	if app_params.Debug > 0 {
		log.Printf("Received SCN from HSM.\n")
		if app_params.Debug > 2 {
			log.Printf("Contents: '%s'\n", string(body))
		}
	}

	jdcMutex.Lock()

	//Is this the first SCN on an empty cache?

	if (len(jdCache.Components) == 0) || (jdCount == 0) {
		jdcDeepCopy(&jdata)
		jdCount++
		jdcMutex.Unlock()
		return
	}

	//Check the SCN for equality to the cache

	if scnCacheEqual(&jdata) {
		jdCache.Components = append(jdCache.Components, jdata.Components...)
		jdCount++

		if jdCount >= app_params.Scn_max_cache {
			jdCache.Timestamp = time.Now().Format(time.RFC3339Nano)
			scnQ <- jdCache
			jdCache = Scn{}
			jdCount = 0
		}
	} else {
		if len(jdCache.Components) > 0 {
			jdCache.Timestamp = time.Now().Format(time.RFC3339Nano)
			scnQ <- jdCache
			jdcDeepCopy(&jdata)
			jdCount = 1
		}
	}

	jdcMutex.Unlock()

	w.WriteHeader(http.StatusOK)
}

// Convenience func, does an Scn struct deep copy.

func jdcDeepCopy(jdp *Scn) {
	jdCache.State = jdp.State
	jdCache.Flag = jdp.Flag
	jdCache.Role = jdp.Role
	jdCache.SubRole = jdp.SubRole
	jdCache.SoftwareStatus = jdp.SoftwareStatus
	if jdp.Enabled != nil {
		enb := *jdp.Enabled
		jdCache.Enabled = &enb
	}

	jdCache.Components = make([]string, len(jdp.Components))
	copy(jdCache.Components, jdp.Components)
}

// Compares an Scn struct to the global cached version.

func scnCacheEqual(jdp *Scn) bool {
	enblp := (jdp.Enabled != nil) && (*jdp.Enabled != false)
	enblc := (jdCache.Enabled != nil) && (*jdCache.Enabled != false)

	if (jdp.State != jdCache.State) ||
		(jdp.Flag != jdCache.Flag) ||
		(jdp.SoftwareStatus != jdCache.SoftwareStatus) ||
		(enblp != enblc) ||
		(jdp.Role != jdCache.Role) {
		return false
	}

	return true
}

// Goroutine that checks the SCN cache periodically.  If the cache is not
// empty, it will get put into the SCN processing Q.  This prevents the cache
// from sitting there if no SCNs are inbound.

func checkSCNCache() {
	for {
		time.Sleep(time.Duration(app_params.Scn_cache_delay) * time.Second)

		jdcMutex.Lock()
		if len(jdCache.Components) > 0 {
			jdCache.Timestamp = time.Now().Format(time.RFC3339Nano)
			scnQ <- jdCache
			jdCache = Scn{}
			jdCount = 0
		}
		jdcMutex.Unlock()
	}
}

// Process the Q of SCNs to be sent to subscribers.

func handleSCNs() {
	for {
		scn := <-scnQ
		sendToTelemetryBus(scn)
		doScn(scn)
		if app_params.Debug > 0 {
			log.Printf("Remaining in Q: %d", len(scnQ))
		}
	}
}

// Do the dirty work of sending SCNs to subscribers.

func doScn(jdata Scn) {
	var jdata_lc Scn
	var prunemap_copy = make(map[string]bool)

	jdata_lc = jdata
	scnToLower(&jdata_lc)

	//Perform a prune operation if this state shows nodes/targets becoming
	//unavailable.  No sense sending anything to a down node.

	prune := isStateUnavailable(jdata)
	if prune {
		//Add all nodes in this SCN into the prune map.  Use this map within this
		//func to avoid sending to unavailable targets.  Eventually we'll clean
		//out the ETCD keys as well, which will reduce ETCD overhead.
		prunemap_mutex.Lock()
		for ix := 0; ix < len(jdata_lc.Components); ix++ {
			prunemap[jdata_lc.Components[ix]] = true
		}
		//Make a copy of the prunemap in case it gets processed/deleted between
		//when we read the KVs from ETCD and when we process them.  Unlikely but
		//possible.
		for k, v := range prunemap {
			prunemap_copy[k] = v
		}
		prunemap_mutex.Unlock()
	}

	//Make a list of SCN attributes

	scnAttrs := getSCNAttrs(jdata_lc)

	//Search for SCN State in the list of subscriptions.  TODO: maybe this needs
	//to be paged for best scaling?

	kvlist, kverr := kvHandle.GetRange(SUBSCRIBER_KEYRANGE_START, SUBSCRIBER_KEYRANGE_END)
	if kverr != nil {
		log.Printf("ERROR: Problem retrieving key list for SCN %s: %v",
			jdata_lc.State, kverr)
		return
	}

	for _, sub := range kvlist {
		attrMatch := false

		for _, attr := range scnAttrs {
			if strings.Contains(sub.Key, attr) {
				//match!
				attrMatch = true
				break
			}
		}

		//Split the key to get the subscriber/xname.
		//The key's value will be the list of nodes this node
		//wants notifications for.

		toks := strings.Split(sub.Key, SUBSCRIBER_KEY_DELIM)
		subxname := toks[SUBSCRIBER_TOKNUM_XNAME]

		//Fan out the SCN if this subscriber hasn't been pruned.

		if attrMatch && !(prune && prunemap_copy[subxname]) {
			//The SCN matches a subscriber's SCN request.  We'll need to
			//to send them a JSON payload with the new state and all of
			//the components which match the ones in the subscriber's
			//request list.

			//First unmarshal the JSON data in the key's value -- this is
			//the list of nodes the subscriber is interested in.

			var nsdata SubData
			var sendData Scn

			sendData.Enabled = jdata.Enabled
			sendData.Role = jdata.Role
			sendData.SubRole = jdata.SubRole
			sendData.SoftwareStatus = jdata.SoftwareStatus
			sendData.State = jdata.State
			sendData.Timestamp = jdata.Timestamp
			//Skip components for now, need to do an intersection first.

			umerr := json.Unmarshal([]byte(sub.Value), &nsdata)
			if umerr != nil {
				log.Printf("ERROR: Problem unmarshalling ETCD key '%s': %v",
					sub.Key, umerr)
				return
			}

			//Now intersect the list of nodes subscriber is interested in
			//with the nodes in the SCN

			sendData.Components = intersect(nsdata.ScnNodes, jdata_lc.Components)
			if len(sendData.Components) < 1 {
				if app_params.Debug > 1 {
					log.Printf("Nothing to send to subscriber '%s'\n", subxname)
				}
			} else {
				if app_params.Debug > 2 {
					//N.B.: DO *NOT* do this on large systems with large SCNs!!!
					//The logging will clog up, and will slow everything way down.

					log.Printf("Sending SCN to node %s with the following nodes:",
						subxname)
					for j := 0; j < len(sendData.Components); j += 1 {
						log.Printf("        %s\n", sendData.Components[j])
					}
				}

				//Send the SCN via the worker pool.  Note the infinite for
				//loop -- this should never block for very long, but even if
				//it does, we're in our own goroutine here, so it won't
				//block anything else.  If for whatever reason it blocks
				//forever, something is horribly wrong and we have bigger
				//fish to fry.

				//TODO: we may want to make a "jobValid" map to indicate
				//jobs that are queued, which gets cleared when the
				//job executes.  If a DELETE or prune operation occurs before
				//the job executes, then the job sees the entry is empty
				//and drops the job on the floor.  This addresses the end
				//case where an SCN is queued for fanout to an endpoint which
				//gets deleted/pruned before the SCN queue executes the job,
				//and prevents the SCN from being delivered.

				jj := NewJobSCNSend(sendData, subxname, nsdata.Url)
				for {
					rv := scnWorkPool.Queue(jj)
					if rv == 0 {
						break
					}
					log.Printf("WARNING: SCN send '%s' blocked due to full Q.\n",
						subxname)
					time.Sleep(500 * time.Millisecond)
				}

				//If we're in testing/fanout sync mode, wait for this SCN
				//send to finish before doing the next one.

				if fanoutSyncMode != 0 {
					if app_params.Debug > 2 {
						log.Printf("INFO: In fanout-sync mode, waiting for SCN send to complete.\n")
					}
					for {
						time.Sleep(500 * time.Microsecond)
						jstat, _ := jj.GetStatus()
						if (jstat == base.JSTAT_COMPLETE) ||
							(jstat == base.JSTAT_ERROR) ||
							(jstat == base.JSTAT_CANCELLED) {
							break
						}
					}
					if app_params.Debug > 2 {
						log.Printf("INFO: SCN sent.\n")
					}
				}
			}
		}
	}
}

/////////////////////////////////////////////////////////////////////////////
// Does the dirty work of a PATCH operation on the /params API endpoint.
//
// w(in):  HTTP response writer.
// r(in):  HTTP request.
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func paramsPatch(w http.ResponseWriter, r *http.Request) {
	errinst := "/" + URL_PARAMS
	body, berr := ioutil.ReadAll(r.Body)

	if app_params.Debug > 2 {
		log.Printf("/params PATCH received: '%s'\n", string(body))
	}

	if berr != nil {
		log.Println("Error on message read:", berr)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Error reading inbound request",
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	//OK, payload is OK.  Set the param values found in it.

	perr := parseParamJson(body, PARAM_PATCH)
	if perr != nil {
		log.Printf("Error parsing parameter JSON: '%s'\n", perr.Error())
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			perr.Error(),
			errinst, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}
	//OK, if we got here, things applied correctly.  Generate a JSON
	//response with the current values of the parameters.

	rparams, err := genCurParamJson()
	if err != nil {
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Failed JSON marshall",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}
	//Set this JSON blob as a key for the KV store so that all
	//instances of this service see it and use the same values of
	//parameters.

	serr := kvHandle.Store(KV_PARAM_KEY, string(rparams))
	if serr != nil {
		log.Println("INTERNAL ERROR storing KV params value ",
			string(rparams), ": ", serr)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Failed KV service STORE operation",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(rparams)
}

/////////////////////////////////////////////////////////////////////////////
// Does the dirty work of a GET operation on the /params API endpoint.
//
// w(in):  HTTP response writer.
// r(in):  HTTP request.
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func paramsGet(w http.ResponseWriter, r *http.Request) {
	errinst := "/" + URL_PARAMS
	rparams, err := genCurParamJson()
	if err != nil {
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Failed JSON marshall",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(rparams)
}

func populateSubinfo(xname string, tt []string, subinfo *ScnSubscribe) {
	switch tt[0] {
	case SUBSCRIBER_KEY_HWS:
		for iy := 1; iy < len(tt); iy++ {
			subinfo.States = append(subinfo.States, tt[iy])
		}
		break
	case SUBSCRIBER_KEY_SWS:
		for iy := 1; iy < len(tt); iy++ {
			subinfo.SoftwareStatus = append(subinfo.SoftwareStatus, tt[iy])
		}
		break
	case SUBSCRIBER_KEY_ENBL:
		enn := new(bool)
		*enn = true
		subinfo.Enabled = enn
		break
	case SUBSCRIBER_KEY_ROLES:
		for iy := 1; iy < len(tt); iy++ {
			subinfo.Roles = append(subinfo.Roles, tt[iy])
		}
		break
	case SUBSCRIBER_KEY_SUBROLES:
		for iy := 1; iy < len(tt); iy++ {
			subinfo.SubRoles = append(subinfo.SubRoles, tt[iy])
		}
		break
	case SUBSCRIBER_KEY_SVC:
		subinfo.Subscriber = tt[1] + SUBSCRIBER_SVC_DELIM + xname
		subinfo.SubscriberComponent = xname
		subinfo.SubscriberAgent = tt[1]
		break
	}
}

/////////////////////////////////////////////////////////////////////////////
// Handle getting a list of current SCN subscriptions via the /subscriptions
// API.  Returns JSON payload of all current subscription data to the caller.
//
// w(in):  HTTP response writer
// r(in):  HTTP request
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func subscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	var sublist SubscriptionList

	errinst := "/" + URL_SUBSCRIPTIONS

	if r.Method != "GET" {
		log.Printf("ERROR: request is not a GET.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Only GET operations supported",
			errinst, http.StatusMethodNotAllowed)
		//It is required to have an "Allow:" header with this error
		w.Header().Add("Allow", "GET")
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	//Formulate a JSON payload from our subscription data.  Get all
	//subscription keys from the KV store, iterate over them, and
	//build up the JSON data.

	kvlist, kverr := kvHandle.GetRange(SUBSCRIBER_KEYRANGE_START,
		SUBSCRIBER_KEYRANGE_END)
	if kverr != nil {
		log.Println("ERROR fetching subscription keys:", kverr)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"KV fetch error",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	for _, item := range kvlist {
		var subinfo ScnSubscribe
		var subkeydata SubData

		//separate the fields of the key to populate a subscription record
		toks := strings.Split(item.Key, SUBSCRIBER_KEY_DELIM)
		subinfo.Subscriber = toks[SUBSCRIBER_TOKNUM_XNAME]

		for ix := SUBSCRIBER_TOKNUM_XNAME + 1; ix < len(toks); ix++ {
			tt := strings.Split(toks[ix], SUBSCRIBER_KEYCAT_DELIM)
			populateSubinfo(toks[SUBSCRIBER_TOKNUM_XNAME], tt, &subinfo)
		}

		//Unmarshal the key's value to get the components

		err := json.Unmarshal([]byte(item.Value), &subkeydata)
		if err != nil {
			fmt.Println("ERROR unmarshalling subscriber key data:", err)
			pdet := base.NewProblemDetails("about:blank",
				"Internal Server Error",
				"JSON unmarshal error",
				errinst, http.StatusInternalServerError)
			base.SendProblemDetails(w, pdet, 0)
			return
		}

		subinfo.Components = subkeydata.ScnNodes
		subinfo.Url = subkeydata.Url
		sublist.SubscriptionList = append(sublist.SubscriptionList, subinfo)
	}

	//Marshal into a byte array

	ba, baerr := json.Marshal(sublist)
	if baerr != nil {
		log.Println("ERROR marshaling subscription list info:", baerr)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"JSON marshal error",
			errinst, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	//Return the JSON payload
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(ba)
}

/////////////////////////////////////////////////////////////////////////////
// Generate the API routes
/////////////////////////////////////////////////////////////////////////////

func newRouter(routes []Route) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		var handler http.Handler
		handler = route.HandlerFunc
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
	}

	// If the 'pprof' build tag is set, then this will register pprof handlers,
	// otherwise this function is stubbed and will do nothing.
	RegisterPProfHandlers(router)

	return router
}

// Create the API route descriptors.

func generateRoutes() Routes {
	v1Ubase := URL_DELIM + URL_BASE + URL_DELIM + URL_V1 + "/"
	v2Ubase := URL_DELIM + URL_BASE + URL_DELIM + URL_V2 + "/"

	return Routes{
		//V1 routes
		Route{"paramsGet",
			strings.ToUpper("Get"),
			v1Ubase + URL_PARAMS,
			paramsGet,
		},
		Route{"paramsPatch",
			strings.ToUpper("Patch"),
			v1Ubase + URL_PARAMS,
			paramsPatch,
		},
		Route{"livenessHandler",
			strings.ToUpper("Get"),
			v1Ubase + URL_LIVENESS,
			livenessHandler,
		},
		Route{"readinessHandler",
			strings.ToUpper("Get"),
			v1Ubase + URL_READINESS,
			readinessHandler,
		},
		Route{"healthHandler",
			strings.ToUpper("Get"),
			v1Ubase + URL_HEALTH,
			healthHandler,
		},
		Route{"subscriptionsHandler",
			strings.ToUpper("Get"),
			v1Ubase + URL_SUBSCRIPTIONS,
			subscriptionsHandler,
		},
		Route{"scnHandler",
			strings.ToUpper("Post"),
			v1Ubase + URL_SCN,
			scnHandler,
		},
		Route{"doSubscribePost",
			strings.ToUpper("Post"),
			v1Ubase + URL_SUBSCRIBE,
			doSubscribePost,
		},
		Route{"doSubscribePatch",
			strings.ToUpper("Patch"),
			v1Ubase + URL_SUBSCRIBE,
			doSubscribePatch,
		},
		Route{"doSubscribeDelete",
			strings.ToUpper("Delete"),
			v1Ubase + URL_SUBSCRIBE,
			doSubscribeDelete,
		},

		//V2 routes
		Route{"paramsGet",
			strings.ToUpper("Get"),
			v2Ubase + URL_PARAMS,
			paramsGet,
		},
		Route{"paramsPatch",
			strings.ToUpper("Patch"),
			v2Ubase + URL_PARAMS,
			paramsPatch,
		},
		Route{"livenessHandler",
			strings.ToUpper("Get"),
			v2Ubase + URL_LIVENESS,
			livenessHandler,
		},
		Route{"readinessHandler",
			strings.ToUpper("Get"),
			v2Ubase + URL_READINESS,
			readinessHandler,
		},
		Route{"healthHandler",
			strings.ToUpper("Get"),
			v2Ubase + URL_HEALTH,
			healthHandler,
		},
		Route{"subscriptionsHandler",
			strings.ToUpper("Get"),
			v2Ubase + URL_SUBSCRIPTIONS,
			subscriptionsHandler,
		},
		Route{"subscriptionsXNameGetHandler",
			strings.ToUpper("Get"),
			v2Ubase + URL_SUBSCRIPTIONS + "/{xname}",
			subscriptionsXNameGetHandler,
		},
		Route{"subscriptionsAgentPostHandler",
			strings.ToUpper("Post"),
			v2Ubase + URL_SUBSCRIPTIONS + "/{xname}/agents/{agent}",
			subscriptionsAgentPostHandler,
		},
		Route{"subscriptionsAgentPatchHandler",
			strings.ToUpper("Patch"),
			v2Ubase + URL_SUBSCRIPTIONS + "/{xname}/agents/{agent}",
			subscriptionsAgentPatchHandler,
		},
		Route{"subscriptionsAgentDeleteHandler",
			strings.ToUpper("Delete"),
			v2Ubase + URL_SUBSCRIPTIONS + "/{xname}/agents/{agent}",
			subscriptionsAgentDeleteHandler,
		},
		Route{"subscriptionsXNameDeleteHandler",
			strings.ToUpper("Delete"),
			v2Ubase + URL_SUBSCRIPTIONS + "/{xname}/agents",
			subscriptionsXNameDeleteHandler,
		},
		Route{"scnHandler",
			strings.ToUpper("Post"),
			v2Ubase + URL_SCN,
			scnHandler,
		},
	}
}
