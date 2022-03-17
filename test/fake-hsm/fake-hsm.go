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
    "net/http"
    "log"
    "os"
    "encoding/json"
    "io/ioutil"
)

type ScnSubscribe struct {
    Subscriber string       `json:"Subscriber"`               //[service@]xname (nodes) or 'hmnfd'
    Components []string     `json:"Components,omitempty"`     //SCN components (usually nodes)
    Url string              `json:"Url"`                      //URL to send SCNs to
    States []string         `json:"States,omitempty"`         //Subscribe to these HW SCNs
    Enabled *bool           `json:"Enabled,omitempty"`        //true==all enable/disable SCNs
    SoftwareStatus []string `json:"SoftwareStatus,omitempty"` //Subscribe to these SW SCNs
    //Flag bool               `json:"Flags,omitempty"`        //Subscribe to flag changes
    Roles []string          `json:"Roles,omitempty"`          //Subscribe to role changes
}



func stateComps(w http.ResponseWriter, r *http.Request) {
	//Return nothing useful.  This is for pruneDeadwood().
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

func subs_rcv(w http.ResponseWriter, r *http.Request) {
    if (r.Method != "POST") {
        log.Printf("ERROR: request is not a POST.\n")
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    agent := r.Header.Get("User-Agent")
    log.Printf("Sender: '%s'",agent)

    var jdata ScnSubscribe
    body,err := ioutil.ReadAll(r.Body)
    err = json.Unmarshal(body,&jdata)
    if (err != nil) {
        log.Println("ERROR unmarshaling data:",err)
        w.WriteHeader(http.StatusBadRequest)
        return
    }

    log.Printf("=================================================\n")
    log.Printf("Received an SCN subscription:\n")
    log.Printf("    Subscriber: %s\n",jdata.Subscriber)
    log.Printf("    Url:        %s\n",jdata.Url)
    if (len(jdata.States) > 0) {
        log.Printf("    States:     '%s'\n",jdata.States[0])
        for ix := 1; ix < len(jdata.States); ix++ {
            log.Printf("                '%s'\n",jdata.States[ix])
        }
    }
    if (len(jdata.SoftwareStatus) > 0) {
        log.Printf("    SWStatus:   '%s'\n",jdata.SoftwareStatus[0])
        for ix := 1; ix < len(jdata.SoftwareStatus); ix++ {
            log.Printf("                '%s'\n",jdata.SoftwareStatus[ix])
        }
    }
    if (len(jdata.Roles) > 0) {
        log.Printf("    Roles:      '%s'\n",jdata.Roles[0])
        for ix := 1; ix < len(jdata.Roles); ix++ {
            log.Printf("                '%s'\n",jdata.Roles[ix])
        }
    }
    if (jdata.Enabled != nil) {
        log.Printf("    Enabled:    %t\n",*jdata.Enabled)
    }
    log.Printf("\n")
    log.Printf("=================================================\n")
    w.WriteHeader(http.StatusOK)
}


func main() {
    var envstr string
    port := "27999"

    envstr = os.Getenv("PORT")
    if (envstr != "") {
        port = envstr
    }

    http.HandleFunc("/hsm/v1/Subscriptions/SCN",subs_rcv)
    http.HandleFunc("/hsm/v1/State/Components",stateComps)
    log.Printf("==> Listening on port '%s'",port)

    err := http.ListenAndServe(":"+port,nil)
    if (err != nil) {
        log.Println("ERROR firing up HTTP:",err)
        os.Exit(1)
    }

    os.Exit(0)
}
