package main

import (
    "net/http"
    "log"
    "fmt"
    "os"
    "encoding/json"
    "io/ioutil"
    "crypto/tls"
    "time"
    "bytes"
    "strings"
    "sync"
    "flag"
)

type Scn struct {
    Components []string   `json:"Components"`
    Enabled *bool         `json:"Enabled,omitempty"`
    //Flag string           `json:"Flag,omitempty"`
    Role string           `json:"Role,omitempty"`
    SubRole string        `json:"SubRole,omitempty"`
    SoftwareStatus string `json:"SoftwareStatus,omitempty"`
    State string          `json:"State,omitempty"`
}

type ScnSubscribe struct {
    Subscriber string       `json:"Subscriber"`
    Components []string     `json:"Components,omitempty"`
    Url string              `json:"Url"`
    States []string         `json:"States,omitempty"`
    Enabled *bool           `json:"Enabled,omitempty"`
    SoftwareStatus []string `json:"SoftwareStatus,omitempty"`
    Roles []string          `json:"Roles,omitempty"`
    SubRoles []string       `json:"SubRoles,omitempty"`
}


type httpStuff struct {
    stuff string
}

var nodename = "x1111c1s1b0n1"
var port = "29000"

var lastScnReceived Scn
var scnMutex = &sync.Mutex{}

var nfdSubscribeUrl = "https://localhost:28600/hmi/v1/subscribe"
var scnNodes = []string{"x0c0s0b0n0","x0c0s0b0n1"}
var scnStates = []string{"Ready"}
var scnSWStatus []string
var scnRoles []string
var scnSubRoles []string
var scnEnabled = false
var myHost = "localhost"



func (p *httpStuff) subs_rcv(w http.ResponseWriter, r *http.Request) {
    if (r.Method != "POST") {
        log.Printf("ERROR: request is not a POST.\n")
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }

    var jdata Scn
    body,err := ioutil.ReadAll(r.Body)
    err = json.Unmarshal(body,&jdata)
    if (err != nil) {
        log.Println("ERROR unmarshaling data:",err)
        w.WriteHeader(http.StatusBadRequest)
        return
    }

    scnMutex.Lock()
    lastScnReceived = jdata
    scnMutex.Unlock()

    log.Printf("##########################################################\n")
    log.Printf("Received an SCN:\n")
    if (jdata.State != "") {
        log.Printf("    State:    '%s'\n", jdata.State)
    }
    if (jdata.SoftwareStatus != "") {
        log.Printf("    SWStatus: '%s'\n", jdata.SoftwareStatus)
    }
    if (jdata.Role != "") {
        log.Printf("    Role:     '%s'\n", jdata.Role)
    }
    if (jdata.SubRole != "") {
        log.Printf("    SubRole:  '%s'\n", jdata.SubRole)
    }
    if (jdata.Enabled != nil) {
        log.Printf("    Enabled:  '%t'\n", *jdata.Enabled)
    }

    for i := 0; i < len(jdata.Components); i++ {
        log.Printf("        Comp[%02d]: '%s'\n",i,jdata.Components[i])
    }
    log.Printf("##########################################################\n")

    w.WriteHeader(http.StatusOK)
}

func (p *httpStuff) lastscn_rcv(w http.ResponseWriter, r *http.Request) {
    if (r.Method != "GET") {
        log.Printf("ERROR: request is not a GET.\n")
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }

    //time.Sleep(2 * time.Second)
    scnMutex.Lock()
    jstr,jerr := json.Marshal(lastScnReceived)
    scnMutex.Unlock()
    if (jerr != nil) {
        log.Println("ERROR marshaling JSON data:",jerr)
        w.WriteHeader(http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type","application/json")
    w.WriteHeader(http.StatusOK)
    w.Write(jstr)
}

func (p *httpStuff) clean(w http.ResponseWriter, r *http.Request) {
    //Don't care what the method is, just do it

    log.Printf("Clearing out last SCN record(s).\n")
    scnMutex.Lock()
    lastScnReceived = Scn{}
    scnMutex.Unlock()
    w.WriteHeader(http.StatusOK)
}

func (p *httpStuff) do_subscribe(w http.ResponseWriter, r *http.Request) {
    //Send subscription

    err := sendNFDSubscription()
    if (err != nil) {
        log.Println("ERROR sending hmnfd subscriptions:",err)
        os.Exit(1)
    }
    w.WriteHeader(http.StatusOK)
}

func sendNFDSubscription() error {
    var tr *http.Transport
    var enbp *bool = nil

    subname := fmt.Sprintf("NodeEmulator@%s",nodename)
    url := fmt.Sprintf("http://%s:%s/%s/scn",myHost,port,nodename)

    enb := true
    if (scnEnabled) {
        enbp = &enb
    }
    scnsub := ScnSubscribe{Subscriber:     subname,
                           Components:     scnNodes,
                           States:         scnStates,
                           SoftwareStatus: scnSWStatus,
                           Roles:          scnRoles,
                           SubRoles:       scnSubRoles,
                           Enabled:        enbp,
                           Url:            url,
                          }

    barr,err := json.Marshal(scnsub)
    if (err != nil) {
        log.Println("INTERNAL ERROR marshalling subscription data:",err)
        return err
    }

    log.Printf("============================================\n")
    log.Printf("Sending subscription to hmnfd\n")
    log.Printf("    URL:  '%s'\n",nfdSubscribeUrl)
    log.Printf("    Data: '%s'\n",string(barr))
    log.Printf("============================================\n")

    //TODO: need cmdline args for cert and key files?  We'll see what
    //shakes out from the whole auth/auth implementation.

    if (nfdSubscribeUrl[0:5] == "https") {
        tr = &http.Transport{
                  TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
              }
    } else {
        tr = &http.Transport{}
    }

    //Note: The timeout utilized here covers the entire transaction.  It
    //can be split into separate timeouts for Dial, TLSHandshake, Response,
    //etc.

    client := &http.Client{Transport: tr,
                           Timeout:   (time.Duration(5) * time.Second),
                          }

    // Make POST request

    req,_ := http.NewRequest("POST", nfdSubscribeUrl, bytes.NewBuffer(barr))
    req.Header.Set("Content-Type","application/json")

    envstr := os.Getenv("CLOSEREQ")
    if (envstr != "") {
        req.Close = true
    }

    rsp,err := client.Do(req)

    if (err != nil) {
        log.Println("ERROR sending POST to hmnfd:",err)
        //TODO: what now?
        return err
    } else {
        defer rsp.Body.Close()
        if ((rsp.StatusCode == http.StatusOK) ||
            (rsp.StatusCode == http.StatusNoContent) ||
            (rsp.StatusCode == http.StatusAccepted)) {
            //Read back the response.

            //TODO: Successfully did the POST, but did SM response show some
            //kind of operational error?  Is this possible?

            log.Printf("SUCCESS sending subscription to hmnfd.\n")
        } else {
            log.Println("ERROR response from hmnfd:",rsp.Status,
                        "Error code:",rsp.StatusCode)
        }
    }

    return nil
}

func parse_cmd_line() {
    unstr := "xxx"

    nodeP := flag.String("nodename",unstr,"Name of subscribing node")
    portP := flag.String("port",unstr,"Port number for subscriber URL")
    nfd_suburlP := flag.String("nfd_suburl",unstr,"URL of hmnfd subscribe API")
    scn_nodesP := flag.String("scn_nodes",unstr,"Comma-separated list of SCN nodes")
    scn_statesP := flag.String("scn_states",unstr,"States to subscribe for")
    scn_swstatusP := flag.String("scn_swstatus",unstr,"Software Status' to subscribe for")
    scn_rolesP := flag.String("scn_roles",unstr,"Node roles to subscribe for")
    scn_subrolesP := flag.String("scn_subroles",unstr,"Node sub-roles to subscribe for")
    scn_enblP := flag.Bool("scn_enable",false,"Subscribe for Enable changes")
    fake_hostP := flag.String("fake_host",unstr,"Hostname to impersonate")

    flag.Parse()

    if (*nodeP != unstr) {
        nodename = *nodeP
    }
    if (*portP != unstr) {
        port = *portP
    }
    if (*nfd_suburlP != unstr) {
        nfdSubscribeUrl = *nfd_suburlP
    }
    if (*scn_nodesP != unstr) {
        scnNodes = strings.Split(*scn_nodesP,",")
    }
    if (*scn_statesP != unstr) {
        scnStates = strings.Split(*scn_statesP,",")
    }
    if (*scn_swstatusP != unstr) {
        scnSWStatus = strings.Split(*scn_swstatusP,",")
    }
    if (*scn_rolesP != unstr) {
        scnRoles = strings.Split(*scn_rolesP,",")
    }
    if (*scn_subrolesP != unstr) {
        scnSubRoles = strings.Split(*scn_subrolesP,",")
    }
    if (*scn_enblP == true) {
        scnEnabled = true
    }
    if (*fake_hostP != unstr) {
        myHost = *fake_hostP
    }
}

func main() {
    var envstr string

    envstr = os.Getenv("NODE")
    if (envstr != "") {
        nodename = envstr
    }
    envstr = os.Getenv("PORT")
    if (envstr != "") {
        port = envstr
    }
    envstr = os.Getenv("NFDSUBURL")
    if (envstr != "") {
        nfdSubscribeUrl = envstr
    }
    envstr = os.Getenv("SUBNODES") //comma separated list
    if (envstr != "") {
        scnNodes = strings.Split(envstr,",")
    }
    envstr = os.Getenv("SUBSTATES") //comma separated list
    if (envstr != "") {
        scnStates = strings.Split(envstr,",")
    }
    envstr = os.Getenv("SUBSWSTATUS") //comma separated list
    if (envstr != "") {
        scnSWStatus = strings.Split(envstr,",")
    }
    envstr = os.Getenv("ROLES") //comma separated list
    if (envstr != "") {
        scnRoles = strings.Split(envstr,",")
    }
    envstr = os.Getenv("SUBROLES") //comma separated list
    if (envstr != "") {
        scnSubRoles = strings.Split(envstr,",")
    }
    envstr = os.Getenv("ENABLED") //comma separated list
    if (envstr != "") {
        scnEnabled = true
    }
    envstr = os.Getenv("FAKEHOST")
    if (envstr != "") {
        myHost = envstr
    }

    parse_cmd_line()

    urlep := "/"+nodename+"/scn"
    url_lastscn := "/"+nodename+"/lastscn"
    url_clean := "/"+nodename+"/clean"
    url_do_sub := "/"+nodename+"/do_subscribe"
    url_front := "http://" + myHost + ":" + port

    hstuff := new(httpStuff)
    http.HandleFunc(urlep,hstuff.subs_rcv)
    log.Printf("Listening on endpoints '%s', '%s', '%s', port '%s'\n",
        url_front+urlep,url_front+url_lastscn,url_front+url_clean,port)

    http.HandleFunc(url_lastscn,hstuff.lastscn_rcv)
    http.HandleFunc(url_clean,hstuff.clean)
    http.HandleFunc(url_do_sub,hstuff.do_subscribe)

    err := http.ListenAndServe(":"+port,nil)
    if (err != nil) {
        log.Println("ERROR firing up HTTP:",err)
        os.Exit(1)
    }

    os.Exit(0)
}

