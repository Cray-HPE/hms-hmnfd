// MIT License
// 
// (C) Copyright [2019-2021] Hewlett Packard Enterprise Development LP
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
    "log"
    "fmt"
    "os"
    "net/http"
    "encoding/json"
    "time"
    "bytes"
    "strings"
    "io/ioutil"
    "stash.us.cray.com/HMS/hms-base"
)

// Used for subscription tracking to insure we don't send > 1
// subscription to HSM for any given thing.

type subTracker struct {
    hwStates map[string]bool
    swStatus map[string]bool
    roles map[string]bool
    subroles map[string]bool
    enabled bool
}

// This is the data stored in the HSM subscription key in ETCD,
// used for auto-discovery on startup.

type hsmSubscriptionInfo struct {
    HWStates []string   `json:"HWStates,omitempty"`
    SWStatus []string   `json:"SWStatus,omitempty"`
    Roles []string      `json:"Roles,omitempty"`
    SubRoles []string   `json:"SubRoles,omitempty"`
    Enabled bool        `json:"Enabled,omitempty"`
}

type hsmStateOnly struct {
    Flag  string `json:"Flag"`
    ID    string `json:"ID"`
    Type  string `json:"Type"`
    State string `json:"State"`
}

type hsmStateArray struct {
    Components []hsmStateOnly
}

var __hsm_scn_subscription_ix int = 1
var hostName string

/////////////////////////////////////////////////////////////////////////////
// Send subscription to the State Manager for State Change Notification.
//
// scn(in):  SCN subscription data.
// Return:   nil on success, error string on error.
/////////////////////////////////////////////////////////////////////////////

func sendHsmScnSubscription(scn ScnSubscribe) error {
    smURL := app_params.SM_url + URL_DELIM + SM_SCN_SUB

    //Remove the Components from the struct, as HSM doesn't
    //use them.  Also, change the Subscriber and URL to hmnfd's.

    scn.Components = nil

    //Use a unique subscriber ID.  It needs to contain the pod instance
    //name plus an offset.

    if (hostName == "") {
		var herr error
        hostName,herr = os.Hostname()
        if (herr != nil) {
            log.Printf("Error getting host name: %v -- using random number.",
                    herr)
            hostName = fmt.Sprintf("%s%d",URL_APPNAME,time.Now().Nanosecond())
        }
    }
    scn.Subscriber = fmt.Sprintf("%s_%d",hostName,__hsm_scn_subscription_ix)
    __hsm_scn_subscription_ix ++
    scn.Url = app_params.Scn_in_url

    barr,err := json.Marshal(scn)
    if (err != nil) {
        log.Println("INTERNAL ERROR marshalling SM subscription req info:",err)
        return err
    }

    if (app_params.Debug > 1) {
        log.Println("Sending POST to State Mgr URL:",smURL,
                "Data:",string(barr))
    }

    //Don't actually send anything to the SM if we're in "--nosm" mode.

    if (app_params.Nosm != 0) {
        return nil
    }

    // Make POST request

    req,qerr := http.NewRequest("POST", smURL, bytes.NewBuffer(barr))
    if (qerr != nil) {
        log.Println("ERROR opening POST request to SM:",qerr)
        return qerr
    }
    defer req.Body.Close()
    req.Header.Set("Content-Type","application/json")
    base.SetHTTPUserAgent(req,serviceName)

    rsp,rerr := htrans.client.Do(req)

    if (rerr != nil) {
        log.Println("ERROR sending POST to SM:",rerr)
        //TODO: what now?
        return rerr
    } else {
        defer rsp.Body.Close()
        if ((rsp.StatusCode == http.StatusOK) ||
            (rsp.StatusCode == http.StatusNoContent) ||
            (rsp.StatusCode == http.StatusAccepted)) {
            //Read back the response.

            //TODO: Successfully did the POST, but did SM response show some
            //kind of operational error?  Is this possible?

            if (app_params.Debug > 0) {
                log.Println("SUCCESS sending POST to SM, response:",rsp)
            }
        } else {
            lerr := fmt.Errorf("ERROR response from State Manager: %s, Error code: %d\n",
                               rsp.Status, rsp.StatusCode)
            return lerr
        }
    }

    return nil
}

/////////////////////////////////////////////////////////////////////////////
// Given a subscription request from a node, check to see if we've subscribed
// to all of its attributes with the State Manager.
//
// sub(in): Subscription request from a node.
// tracker(inout): HSM subscription attribute tracking data.
// Return:         true if we need to do an HSM subscription to cover a new
//                     attribute, else false.
/////////////////////////////////////////////////////////////////////////////

func needHSMSubs(sub ScnSubscribe, tracker subTracker) (bool,subTracker) {
    ttmp := tracker
    needSub := false

    for _,state := range(sub.States) {
        stl := strings.ToLower(state)
        _,ok := tracker.hwStates[stl]
        if (!ok) {
            ttmp.hwStates[stl] = true
            needSub = true
        }
    }
    for _,sws := range(sub.SoftwareStatus) {
        stl := strings.ToLower(sws)
        _,ok := tracker.swStatus[stl]
        if (!ok) {
            ttmp.swStatus[stl] = true
            needSub = true
        }
    }
    for _,role := range(sub.Roles) {
        stl := strings.ToLower(role)
        _,ok := tracker.roles[stl]
        if (!ok) {
            ttmp.roles[stl] = true
            needSub = true
        }
    }
    for _,subrole := range(sub.SubRoles) {
        stl := strings.ToLower(subrole)
        _,ok := tracker.subroles[stl]
        if (!ok) {
            ttmp.subroles[stl] = true
            needSub = true
        }
    }
    if (!tracker.enabled && (sub.Enabled != nil) &&
       (*sub.Enabled == true)) {
        ttmp.enabled = true
        needSub = true
    }

    return needSub,ttmp
}

/////////////////////////////////////////////////////////////////////////////
// Given our HSM subscription tracking data, create a subscription request
// for the HSM.  Note that this will always be additive over the previous
// subscription; therefore it is important that the HSM coalesce/dedup the
// subscriptions it maintains so that we only get one SCN delivered at a 
// time.
//
// tracker(in): HSM subscription tracking data.
// Return:      JSON string to send to HSM for subscription; nil on success,
//                 error message on error (marshal error)
/////////////////////////////////////////////////////////////////////////////

func makeHSMSubInfo(tracker *subTracker) (string,error) {
    var hsi hsmSubscriptionInfo

    for key,_ := range(tracker.hwStates) {
        hsi.HWStates = append(hsi.HWStates,key)
    }
    for key,_ := range(tracker.swStatus) {
        hsi.SWStatus = append(hsi.SWStatus,key)
    }
    for key,_ := range(tracker.roles) {
        hsi.Roles = append(hsi.Roles,key)
    }
    for key,_ := range(tracker.subroles) {
        hsi.SubRoles = append(hsi.SubRoles,key)
    }
    hsi.Enabled = tracker.enabled

    jstr,err := json.Marshal(hsi)
    if (err != nil) {
        return "",err
    }

    return string(jstr),nil
}

/////////////////////////////////////////////////////////////////////////////
// Time based loop to handle State Manager SCN subscriptions.  This is used
// to batch subscriptions up and to prevent duplicates/noise.
//
// Args, Return: None.
/////////////////////////////////////////////////////////////////////////////

func subscribeToHsmScn() {
    var tracker = subTracker{hwStates: make(map[string]bool),
                             swStatus: make(map[string]bool),
                             roles:    make(map[string]bool),
                             subroles: make(map[string]bool),
                             enabled: false,
                            }

    log.Printf("Subscriber loop started.\n")

    for {
        select {
            case sub := <- hsmsub_chan:
                needSub,tracker_tmp := needHSMSubs(sub,tracker)
                if (!needSub) {
                    continue
                }
                if (app_params.Nosm == 0) {
                    log.Println("Sending SCN subscription to HSM:",sub)

                    //This is new, send SCN subscription req to HSM
                    err := sendHsmScnSubscription(sub)
                    if (err != nil) {
                        log.Println("ERROR sending HSM SCN subscription:",err,"retrying.")
                        time.Sleep(time.Second)
                        hsmsub_chan <- sub
                        continue
                    }
                    log.Printf("HSM subscription sent.\n")
                }

                tracker = tracker_tmp

                //Update an ETCD key with current subscription info.

                for ix := 1; ix < 3; ix ++ {
                    jstr,err := makeHSMSubInfo(&tracker)
                    if (err != nil) {
                        log.Println("ERROR! Can't marshal HSM SCN subscription tracking data:",err)
                    } else {
                        kerr := kvHandle.Store(HSM_SUBS_KEY,jstr)
                        if (kerr != nil) {
                            log.Println("ERROR storing SCN subscription indicator in ETCD:",
                                kerr,"(attempt",ix,")")
                        }
                    }
                }
        }
    }
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to create a subscription key/value in ETCD.
//
// complist(in):  Component list, from SCN subscription.
// url(in):       URL to send SCN to, from SCN subscription.
// key(in):       Subscription key to use in ETCD to store this info.
// Return:        nil on success, error string on error.
/////////////////////////////////////////////////////////////////////////////

func makeSubscriptionEntry(complist []string, url string, key string) error {
    var sd SubData
    sd.Url = url
    sd.ScnNodes = complist

    //Marshal
    jstr,jerr := json.Marshal(sd)
    if (jerr != nil) {
        return jerr
    }
    if (app_params.Debug > 2) {
        log.Printf("Storing subscription info, key: '%s'\n",key)
        log.Printf("    value: '%s'\n",string(jstr))
    }
    err := kvHandle.Store(key,string(jstr))
    if (err != nil) {
        return err
    }
    return nil
}

/////////////////////////////////////////////////////////////////////////////
// Send an SCN to a subscriber.  This is called by the worker pool -- don't
// call directly!  This function will attempt a few times, and if it 
// consistently fails, the subscription will be pruned.
//
// sd(in):         SCN data to send.
// subscriber(in): Subscriber XName to send to.
// url(in):        URL to send SCN to.
// Return:         None.
/////////////////////////////////////////////////////////////////////////////

func sendSCNToSubscriber(sd Scn, subscriber string, url string) {
    var retry int
    var prune bool = false

    //For testing purposes.
    if (url == "") {
        return
    }

    //Don't send if we've been pruned.

    prunemap_mutex.Lock()
    prune = prunemap[subscriber]
    prunemap_mutex.Unlock()
    if (prune) {
        if (app_params.Debug > 0) {
            log.Printf("Not sending SCN to '%s'/'%s', node has been pruned.\n",
                subscriber,url)
        }
        return
    }

    ba,berr := json.Marshal(sd)
    if (berr != nil) {
        log.Println("ERROR marshaling json data:",berr)
        return
    }

   //TODO: this connection should be kept open, not sent each time if we
   //have to use HTTPS, which has expensive overhead with each new connection.

    for retry = 1; retry <= SCN_SEND_RETRIES; retry++ {
        req,err := http.NewRequest("POST",url,bytes.NewBuffer(ba))
        if (err != nil) {
            log.Println("ERROR creating HTTP POST request to url:",url,":",err)
            continue
        }
        req.Header.Set("Content-Type","application/json")
        base.SetHTTPUserAgent(req,serviceName)
        req.Close = true
        rsp,err := htrans.client.Do(req)

        if (err != nil) {
            //This is hokey, but there's no other way to get the type of
            //error.  ECONNREFUSED means the client is gone.  Prune it.
            estr := strings.ToLower(err.Error())
            if (strings.Contains(estr,"connection refused")) {
                log.Printf("Connection refused for '%s', dropping.",url)
                prunemap_mutex.Lock()
                prunemap[subscriber] = true
                prunemap_mutex.Unlock()
                return
            }

            log.Printf("ERROR sending SCN (attempt #%d), to '%s': %s",
                    retry,url,err.Error())
            continue
        }

        rsp.Body.Close()

        //Check response code, should be 200
        if (rsp.StatusCode == http.StatusOK) {
            if (retry > 1) {
                log.Printf("INFO: SCN send to '%s' succeeded (attempt #%d).",
                    url,retry)
            }
            break
        } else {
            log.Printf("ERROR response sending SCN (attempt #%d), to '%s', status code %d:",
                retry,url,rsp.StatusCode)
        }
    }

    if (retry >= 4) {
        log.Printf("Maximum retries exhausted, dropping subscription for '%s'/'%s'\n",
            subscriber,url)
        //Prune this subscriber
        prunemap_mutex.Lock()
        prunemap[subscriber] = true
        prunemap_mutex.Unlock()
    } else {
        if (app_params.Debug > 1) {
            log.Printf("Sent SCN to subscriber '%s' at '%s'\n",
                    subscriber,url)
        }
    }
}

// This is run on startup.  Grab the subscription keys, then grab all of
// the node components from HSM.  Any HSM components in an unavailable state
// will have their subscriptions pruned.

func pruneDeadWood() {
    var jdata hsmStateArray

    if (app_params.Nosm != 0) {
        return
    }

    badMap := make(map[string]bool)
    smURL := app_params.SM_url + URL_DELIM + SM_STATEDATA

    //Get HSM states of all nodes

    for {
        req,err := http.NewRequest("GET",smURL,nil)
        if (err != nil) {
            log.Printf("ERROR creating HTTP POST request to url '%s': %v",
                smURL,err)
            time.Sleep(2 * time.Second)
            continue
        }

        q := req.URL.Query()
        q.Add("type","Node")
        q.Add("state","Off")
        q.Add("state","Empty")
        q.Add("state","Halt")
        q.Add("stateonly","true")
        req.URL.RawQuery = q.Encode()
        req.Close = true
        base.SetHTTPUserAgent(req,serviceName)

        rsp,rerr := htrans.client.Do(req)
        if (rerr != nil) {
            log.Printf("ERROR sending GET to HSM for node states: %v",rerr)
            time.Sleep(2 * time.Second)
            continue
        }

        body,berr := ioutil.ReadAll(rsp.Body)
        if (berr != nil) {
            log.Printf("ERROR reading HSM response for node states: %v",berr)
            time.Sleep(2 * time.Second)
            continue
        }

        err = json.Unmarshal(body,&jdata)
        if (err != nil) {
            log.Printf("ERROR unmarshalling HSM response for node states: %v",err)
            time.Sleep(2 * time.Second)
            continue
        }

        //Make a map of bad-state component xnames

        for _,sdata := range(jdata.Components) {
            badMap[sdata.ID] = true
        }
        break
    }

    //Now iterate the list of subscription keys and prune any matches.

    kvlist,kverr := kvHandle.GetRange(SUBSCRIBER_KEYRANGE_START,
                                      SUBSCRIBER_KEYRANGE_END)

    if (kverr != nil) {
        log.Println("ERROR retrieving SCN key list:", kverr)
        return
    }

    var match bool

    for _,sub := range kvlist {
        toks := strings.Split(sub.Key,SUBSCRIBER_KEY_DELIM)
        xname := toks[SUBSCRIBER_TOKNUM_XNAME]
        _,match = badMap[xname]
        if (match) {
            log.Printf("INFO: Pruning  dead node subscription for '%s'",xname)
            prunemap_mutex.Lock()
            prunemap[xname] = true
            prunemap_mutex.Unlock()
        }
    }
}

