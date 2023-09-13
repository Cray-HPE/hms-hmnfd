// MIT License
//
// (C) Copyright [2019,2021,2023] Hewlett Packard Enterprise Development LP
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
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/Cray-HPE/hms-base"
	"github.com/Cray-HPE/hms-hmetcd"
)

var gofuncsRunning = false
var scnsRcv []Scn

func kvPurge(t *testing.T) {
	// make sure something is here to work with
	if kvHandle == nil {
		return
	}
	kvlist, err := kvHandle.GetRange("a", "z")
	if err != nil {
		t.Errorf("ERROR, can't get key range!!\n")
	}

	for _, kv := range kvlist {
		derr := kvHandle.Delete(kv.Key)
		if derr != nil {
			t.Errorf("ERROR, can't delete key '%s'\n", kv.Key)
		}
	}

	kvHandle.Close()
}

func TestScnSubscribeHandler(t *testing.T) {
	var kverr error
	var kvalue string
	var kok bool
	var kvval SubData

	disable_logs()

	//Set up ETCD

	kvHandle, kverr = hmetcd.Open("mem:", "")
	if kverr != nil {
		t.Fatal("KV/ETCD open failed:", kverr)
	}
	defer kvPurge(t)

	//Create 3 fake HTTP posts, one each for POST, PATCH, DELETE

	subkey1 := "sub#x0c1s2b0n3#hs.ready.standby#ss.admindown#enbl.enbl#roles.compute#subroles.ncn-m.ncn-w#svc.handler"

	subid := "handler@x0c1s2b0n3"
	comp1 := "x1000c2s3b0n4"
	comp2 := "x1000c2s3b0n5"
	state1 := "Ready"
	state2 := "Standby"
	url := "http://x0c1s2b0n3:8888/scn"
	bs1 := fmt.Sprintf("{\"Subscriber\":\"%s\",\"Components\":[\"%s\",\"%s\"],\"States\":[\"%s\",\"%s\"],\"SoftwareStatus\":[\"AdminDown\"],\"Roles\":[\"Compute\"],\"SubRoles\":[\"ncn-m\",\"ncn-w\"],\"Enabled\":true,\"Url\":\"%s\"}",
		subid, comp1, comp2, state1, state2, url)
	req1_payload := bytes.NewBufferString(bs1)
	req1, err1 := http.NewRequest("POST", "http://localhost:8080/hmnfd/v1/subscribe",
		req1_payload)
	if err1 != nil {
		t.Fatal(err1)
	}
	rr1 := httptest.NewRecorder()
	handler1 := http.HandlerFunc(doSubscribePost)

	bs3 := fmt.Sprintf("{\"Subscriber\":\"%s\",\"Url\":\"%s\"}",
		subid, url)
	req3_payload := bytes.NewBufferString(bs3)
	req3, err3 := http.NewRequest("DELETE", "http://localhost:8080/hmnfd/v1/subscribe",
		req3_payload)
	if err3 != nil {
		t.Fatal(err3)
	}
	rr3 := httptest.NewRecorder()
	handler3 := http.HandlerFunc(doSubscribeDelete)

	subkey4 := "sub#x0c1s2b0n3#hs.off#svc.handler"
	comp4_1 := "x2000c2s2b0n2"
	comp4_2 := "x3000c3s3b0n3"
	state4_1 := "Off"
	bs4 := fmt.Sprintf("{\"Subscriber\":\"%s\",\"Components\":[\"%s\",\"%s\"],\"States\":[\"%s\"],\"Url\":\"%s\"}",
		subid, comp4_1, comp4_2, state4_1, url)
	req4_payload := bytes.NewBufferString(bs4)
	req4, err4 := http.NewRequest("PATCH", "http://localhost:8080/hmnfd/v1/subscribe",
		req4_payload)
	if err4 != nil {
		t.Fatal(err4)
	}
	rr4 := httptest.NewRecorder()
	handler4 := http.HandlerFunc(doSubscribePatch)

	//Mock up the POST operation

	handler1.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("HTTP handler 'doSubscribePatch' returned bad error code, got %v, want %v\n",
			rr1.Code, http.StatusOK)
	}

	//Check the result.  There should be 1 record in ETCD.

	kvalue, kok, kverr = kvHandle.Get(subkey1)
	if kverr != nil {
		t.Fatal("KV key(1) fetch failed:", kverr)
	}
	if !kok {
		t.Fatal("KV record(1) incorrectly not created for subscription.")
	}
	kverr = json.Unmarshal([]byte(kvalue), &kvval)
	if kverr != nil {
		t.Fatal("ERROR unmarshalling KV JSON value:", kverr)
	}
	if kvval.Url != url {
		t.Errorf("URL Mismatch in subscription record, exp: '%s', got: '%s'\n",
			url, kvval.Url)
	}
	if len(kvval.ScnNodes) != 2 {
		t.Errorf("Incorrect number of nodes in subscription record, exp: 2, got: %d\n",
			len(kvval.ScnNodes))
	} else {
		if kvval.ScnNodes[0] != comp1 {
			t.Errorf("Component 1 mismatch in subscription record, exp: '%s', got: '%s'\n",
				comp1, kvval.ScnNodes[0])
		}
		if kvval.ScnNodes[1] != comp2 {
			t.Errorf("Component 2 mismatch in subscription record, exp: '%s', got: '%s'\n",
				comp2, kvval.ScnNodes[1])
		}
	}

	//Mock up a PATCH operation

	handler4.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("HTTP handler 'doSubscribePatch' returned bad error code, got %v, want %v\n",
			rr4.Code, http.StatusOK)
	}

	//Check the result.  There should be 1 records in ETCD.

	kvalue, kok, kverr = kvHandle.Get(subkey4)
	if kverr != nil {
		t.Fatal("KV 'ready' key fetch failed:", kverr)
	}
	if !kok {
		t.Fatal("KV 'ready' record incorrectly not created for subscription.")
	}
	kverr = json.Unmarshal([]byte(kvalue), &kvval)
	if kverr != nil {
		t.Fatal("ERROR unmarshalling KV JSON value:", kverr)
	}
	if kvval.Url != url {
		t.Errorf("URL Mismatch in subscription record, exp: '%s', got: '%s'\n",
			url, kvval.Url)
	}
	if len(kvval.ScnNodes) != 2 {
		t.Errorf("Incorrect number of nodes in subscription record, exp: 2, got: %d\n",
			len(kvval.ScnNodes))
	} else {
		if kvval.ScnNodes[0] != comp4_1 {
			t.Errorf("Component 1 mismatch in subscription record, exp: '%s', got: '%s'\n",
				comp4_1, kvval.ScnNodes[0])
		}
		if kvval.ScnNodes[1] != comp4_2 {
			t.Errorf("Component 2 mismatch in subscription record, exp: '%s', got: '%s'\n",
				comp4_2, kvval.ScnNodes[1])
		}
	}

	//Mock up the DELETE operation

	handler3.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("HTTP handler 'doSubscribeDelete' returned bad error code, got %v, want %v\n",
			rr3.Code, http.StatusOK)
	}

	//The KV keys should all be gone

	kvalue, kok, kverr = kvHandle.Get(subkey1)
	if kverr != nil {
		t.Fatal("KV GET 1 (after http DELETE) operation failed:", kverr)
	}
	if kok {
		t.Errorf("KV key '%s' still exists, should have been deleted.\n", subkey1)
	}
	kvalue, kok, kverr = kvHandle.Get(subkey4)
	if kverr != nil {
		t.Fatal("KV GET 2 (after http DELETE) operation failed:", kverr)
	}
	if kok {
		t.Errorf("KV key '%s' still exists, should have been deleted.\n", subkey4)
	}
}

func TestScnHandler(t *testing.T) {
	var kverr error
	var subdata SubData
	var hsmscn Scn

	disable_logs()
	if scnWorkPool == nil {
		scnWorkPool = base.NewWorkerPool(10, 10)
		scnWorkPool.Run()
	}
	if !gofuncsRunning {
		go handleSCNs()
		go checkSCNCache()
	}

	//Shortcut: create ETCD entries for subscriptions.  Use "" for URLs in
	//those subscriptions, so that the scn_rcv() function won't try to send
	//to any actual endpoint.

	kvHandle, kverr = hmetcd.Open("mem:", "")
	if kverr != nil {
		t.Fatal("KV/ETCD open failed:", kverr)
	}
	defer kvPurge(t)

	subdata.Url = ""
	subdata.ScnNodes = []string{"x0c1s2b0n3", "x0c2s3b0n4", "x0c3s4b0n5", "x0c4s5b0n6"}
	ba, baerr := json.Marshal(subdata)
	if baerr != nil {
		t.Fatal("Error marshalling JSON.")
	}

	key := "sub#x0c0s0b0n0#hs.ready"
	kverr = kvHandle.Store(key, string(ba))
	if kverr != nil {
		t.Fatal("ERROR storing KV key in ETCD store.")
	}

	//Create the SCN data. TODO: there should be 4 of these, one for
	//each SCN type.

	hsmscn.Components = []string{"x0c2s3b0n4"}
	hsmscn.State = "Ready"
	ba, baerr = json.Marshal(hsmscn)
	if baerr != nil {
		t.Fatal("Error marshalling JSON.")
	}

	//Create fake HTTP stuff

	req1_payload := bytes.NewBuffer(ba)
	req1, err1 := http.NewRequest("POST", "http://localhost:8080/hmnfd/v1/scn",
		req1_payload)
	if err1 != nil {
		t.Fatal(err1)
	}
	rr1 := httptest.NewRecorder()
	handler1 := http.HandlerFunc(scnHandler)
	handler1.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("POST operation failed, got response code %v\n", rr1.Code)
	}

	//test invalid operations

	req2_payload := bytes.NewBufferString("")
	req2, err2 := http.NewRequest("PATCH", "http://localhost:8080/hmnfd/v1/scn",
		req2_payload)
	if err2 != nil {
		t.Fatal(err2)
	}
	rr2 := httptest.NewRecorder()
	handler2 := http.HandlerFunc(scnHandler)
	handler2.ServeHTTP(rr2, req2)
	if rr2.Code == http.StatusOK {
		t.Errorf("Disallowed PATCH operation didn't fail, got response code %v\n",
			rr2.Code)
	}

	req3_payload := bytes.NewBufferString("")
	req3, err3 := http.NewRequest("GET", "http://localhost:8080/hmnfd/v1/scn",
		req3_payload)
	if err3 != nil {
		t.Fatal(err3)
	}
	rr3 := httptest.NewRecorder()
	handler3 := http.HandlerFunc(scnHandler)
	handler3.ServeHTTP(rr3, req3)
	if rr3.Code == http.StatusOK {
		t.Errorf("Disallowed GET operation didn't fail, got response code %v\n",
			rr3.Code)
	}

	req4_payload := bytes.NewBufferString("")
	req4, err4 := http.NewRequest("DELETE", "http://localhost:8080/hmnfd/v1/scn",
		req4_payload)
	if err4 != nil {
		t.Fatal(err4)
	}
	rr4 := httptest.NewRecorder()
	handler4 := http.HandlerFunc(scnHandler)
	handler4.ServeHTTP(rr4, req4)
	if rr4.Code == http.StatusOK {
		t.Errorf("Disallowed DELETE operation didn't fail, got response code %v\n",
			rr4.Code)
	}
}

func subscriberSCNHandler(w http.ResponseWriter, req *http.Request) {
	var jdata Scn

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Printf("ERROR subscriberSCNHandler() reading request body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = json.Unmarshal(body, &jdata)
	if err != nil {
		log.Printf("ERROR subscriberSCNHandler() unmarshaling json body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	scnsRcv = append(scnsRcv, jdata)
	w.WriteHeader(http.StatusOK)
}

func scnCompare(sent, rcv []Scn) string {
	if len(sent) != len(rcv) {
		return fmt.Sprintf("SCN sent/rcv length mismatch: %d/%d",
			len(sent), len(rcv))
	}

	for ix := 0; ix < len(sent); ix++ {
		enbls := (sent[ix].Enabled != nil) && (*sent[ix].Enabled != false)
		enblr := (rcv[ix].Enabled != nil) && (*rcv[ix].Enabled != false)

		if (sent[ix].State != rcv[ix].State) ||
			(sent[ix].Flag != rcv[ix].Flag) ||
			(sent[ix].SoftwareStatus != rcv[ix].SoftwareStatus) ||
			(enbls != enblr) ||
			(sent[ix].Role != rcv[ix].Role) {
			return fmt.Sprintf("SCN mismatch: sent: '%v', rcv: '%v'",
				sent[ix], rcv[ix])
		}

		if len(sent[ix].Components) != len(rcv[ix].Components) {
			return fmt.Sprintf("SCN mismatch, sent/rcv component counts: %d/%d, sent: '%v'",
				len(sent[ix].Components), len(rcv[ix].Components),
				sent[ix])
		}

		sort.Strings(sent[ix].Components)
		sort.Strings(rcv[ix].Components)
		for iy := 0; iy < len(sent[ix].Components); iy++ {
			if sent[ix].Components[iy] != rcv[ix].Components[iy] {
				return fmt.Sprintf("SCN component mismatch, sent/rcv: '%v'/'%v'",
					sent[ix].Components, rcv[ix].Components)
			}
		}
	}

	return ""
}

func sendScn(t *testing.T, hsmscn Scn) {
	ba, baerr := json.Marshal(hsmscn)
	if baerr != nil {
		t.Fatal("Error marshalling JSON.")
	}
	req1_payload := bytes.NewBuffer(ba)
	req1, err1 := http.NewRequest("POST", "http://localhost:8080/hmnfd/v1/scn",
		req1_payload)
	if err1 != nil {
		t.Fatal(err1)
	}
	rr1 := httptest.NewRecorder()
	handler1 := http.HandlerFunc(scnHandler)
	handler1.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("POST operation failed, got response code %v\n", rr1.Code)
	}
}

func TestScnHandler2(t *testing.T) {
	var kverr error
	var subdata SubData
	var hsmscn Scn
	var scnList []Scn
	var cmpStr string

	disable_logs()
	if scnWorkPool == nil {
		scnWorkPool = base.NewWorkerPool(10, 10)
		scnWorkPool.Run()
	}
	if !gofuncsRunning {
		go handleSCNs()
		go checkSCNCache()
	}
	if htrans.transport == nil {
		htrans.transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		htrans.client = &http.Client{Transport: htrans.transport,
			Timeout: (time.Duration(app_params.SM_timeout) *
				time.Second),
		}
	}

	//Shortcut: create ETCD entries for subscriptions.  Use "" for URLs in
	//those subscriptions, so that the scn_rcv() function won't try to send
	//to any actual endpoint.

	kvHandle, kverr = hmetcd.Open("mem:", "")
	if kverr != nil {
		t.Fatal("KV/ETCD open failed:", kverr)
	}
	defer kvPurge(t)

	nwpServer := httptest.NewServer(http.HandlerFunc(subscriberSCNHandler))

	subdata.Url = nwpServer.URL
	for ix := 0; ix < 20; ix++ {
		snode := fmt.Sprintf("x%dc0s0b0n0", ix)
		subdata.ScnNodes = append(subdata.ScnNodes, snode)
	}
	ba, baerr := json.Marshal(subdata)
	if baerr != nil {
		t.Fatal("Error marshalling JSON.")
	}

	key := "sub#x0c0s0b0n0#hs.ready.on"
	kverr = kvHandle.Store(key, string(ba))
	if kverr != nil {
		t.Fatal("ERROR storing KV key in ETCD store.")
	}

	//Sync the scn consumer as best we can by changing the period to 1 second
	//and waiting a while, then changing it to the target frequency.

	app_params.Scn_cache_delay = 1
	app_params.Scn_max_cache = 4
	time.Sleep(10)
	app_params.Scn_cache_delay = 10

	//1. 2 SCNs of the same type, wait for the cache send

	scnsRcv = []Scn{}
	scnList = []Scn{}
	hsmscn.Components = []string{}
	scnList = append(scnList, Scn{State: "Ready"})
	for ix := 0; ix < 2; ix++ {
		cmp := fmt.Sprintf("x%dc0s0b0n0", ix)
		hsmscn.Components = []string{cmp}
		hsmscn.State = "Ready"
		scnList[0].Components = append(scnList[0].Components, cmp)
		sendScn(t, hsmscn)
	}
	time.Sleep(12 * time.Second)
	cmpStr = scnCompare(scnList, scnsRcv)
	if cmpStr != "" {
		t.Errorf("SCN Miscompare: %s", cmpStr)
	}

	//2. 5 SCNs of the same type, cache (of 4) send, then single cached send.

	hsmscn.Components = []string{}
	scnsRcv = []Scn{}
	scnList = []Scn{}
	scnList = append(scnList, Scn{State: "Ready"})
	for ix := 0; ix < 5; ix++ {
		cmp := fmt.Sprintf("x%dc0s0b0n0", ix)
		hsmscn.Components = []string{cmp}
		hsmscn.State = "Ready"
		sendScn(t, hsmscn)
	}
	for ix := 0; ix < 4; ix++ {
		cmp := fmt.Sprintf("x%dc0s0b0n0", ix)
		scnList[0].Components = append(scnList[0].Components, cmp)
	}
	scnList = append(scnList, Scn{State: "Ready"})
	scnList[1].Components = append(scnList[1].Components, "x4c0s0b0n0")
	scnList[1].Components = append(scnList[1].Components, "x10c0s0b0n0")

	hsmscn.Components = []string{"x10c0s0b0n0"}
	hsmscn.State = "Ready"
	sendScn(t, hsmscn)
	time.Sleep(10 * time.Second)
	cmpStr = scnCompare(scnList, scnsRcv)
	if cmpStr != "" {
		t.Errorf("SCN Miscompare: %s", cmpStr)
	}

	//3. 2 SCNs of the same type, 1 SCN of a different type.  Cache send,
	//   then single cached send.

	hsmscn.Components = []string{}
	scnsRcv = []Scn{}
	scnList = []Scn{}
	scnList = append(scnList, Scn{State: "Ready"})
	for ix := 0; ix < 2; ix++ {
		cmp := fmt.Sprintf("x%dc0s0b0n0", ix)
		hsmscn.Components = []string{cmp}
		hsmscn.State = "Ready"
		scnList[0].Components = append(scnList[0].Components, cmp)
		sendScn(t, hsmscn)
	}
	hsmscn.Components = []string{"x10c0s0b0n0"}
	hsmscn.State = "On"
	scnList = append(scnList, Scn{State: "On"})
	scnList[1].Components = []string{"x10c0s0b0n0"}
	sendScn(t, hsmscn)
	time.Sleep(10 * time.Second)
	cmpStr = scnCompare(scnList, scnsRcv)
	if cmpStr != "" {
		t.Errorf("SCN Miscompare: %s", cmpStr)
	}

	nwpServer.Close()
}

func TestSubscriptionsHandler(t *testing.T) {
	var key string
	var kverr error
	var subslist SubscriptionList
	var subdata, subdata2 SubData

	disable_logs()

	//Shortcut: stuff the ETCD KV with subscriptions, then use the func to
	//read them out.

	kvHandle, kverr = hmetcd.Open("mem:", "")
	if kverr != nil {
		t.Fatal("KV/ETCD open failed:", kverr)
	}
	defer kvPurge(t)

	subdata.Url = "a.b.c.d"
	subdata.ScnNodes = []string{"x1c1s1b0n1", "x2c2s2b0n2"}
	ba, baerr := json.Marshal(subdata)
	if baerr != nil {
		t.Fatal("Error marshalling JSON.")
	}

	subdata2.Url = "e.f.g.h"
	subdata2.ScnNodes = []string{"x3c3s3b0n3", "x4c4s4b0n4"}
	ba2, baerr2 := json.Marshal(subdata2)
	if baerr2 != nil {
		t.Fatal("Error marshalling JSON.")
	}

	key = "sub#x0c0s0b0n0#hs.ready.standby"
	kverr = kvHandle.Store(key, string(ba))
	if kverr != nil {
		t.Fatal("ERROR storing KV key in ETCD store.")
	}
	key = "sub#x100c0s0b0n0#hs.off#ss.admindown#roles.compute#subroles.ncn-m.ncn-w#enbl.enbl#svc.handler"
	kverr = kvHandle.Store(key, string(ba2))
	if kverr != nil {
		t.Fatal("ERROR storing KV key in ETCD store.")
	}

	req1_payload := bytes.NewBufferString("")
	req1, err1 := http.NewRequest("GET", "http://localhost:8080/hmnfd/v1/subscriptions",
		req1_payload)
	if err1 != nil {
		t.Fatal(err1)
	}
	rr1 := httptest.NewRecorder()
	handler1 := http.HandlerFunc(subscriptionsHandler)
	handler1.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatal("ERROR, GET request for subscriptions failed:", rr1.Code)
	}

	//Interpret the returned payload

	body, err := ioutil.ReadAll(rr1.Body)
	if err != nil {
		t.Fatal("ERROR reading GET response body:", err)
	}
	err = json.Unmarshal(body, &subslist)
	if err != nil {
		t.Fatal("ERROR unmarshalling GET response body:", err)
	}

	//Since there is an array of subscription records, we don't know
	//what order they will be in after unmarshalling.  So, we'll key
	//them by their URL field.

	six_0 := 0
	six_1 := 1
	if subslist.SubscriptionList[0].Url == "e.f.g.h" {
		six_1 = 0
		six_0 = 1
	}

	if subslist.SubscriptionList[six_0].Subscriber != "x0c0s0b0n0" {
		t.Errorf("Mismatching subscriber: expecting 'x0c0s0b0n0', got '%s'\n",
			subslist.SubscriptionList[six_0].Subscriber)
	}
	//NOTE: these are sorted lexicographically by the unmarshaller.  So make
	//sure they are in lex. order when defining them above, otherwise we have
	//to sort them here.  Ick.  Same for states!!

	for ix, cc := range subdata.ScnNodes {
		if subslist.SubscriptionList[six_0].Components[ix] != cc {
			t.Errorf("Mismatching subscriber[0] component[%d], expecting '%s', got '%s'\n",
				ix, cc, subslist.SubscriptionList[six_0].Components[ix])
		}
	}
	if subslist.SubscriptionList[six_0].States[0] != "ready" {
		t.Errorf("Mismatching subscriber[0] state 0, expecting 'ready', got '%s'\n",
			subslist.SubscriptionList[six_0].States[0])
	}
	if subslist.SubscriptionList[six_0].States[1] != "standby" {
		t.Errorf("Mismatching subscriber[0] state 1, expecting 'standby', got '%s'\n",
			subslist.SubscriptionList[six_0].States[1])
	}
	if subslist.SubscriptionList[six_0].Url != "a.b.c.d" {
		t.Errorf("Mismatching subscriber[0] URL, expecting 'a.b.c.d', got '%s'\n",
			subslist.SubscriptionList[six_0].Url)
	}
	if subslist.SubscriptionList[six_0].Enabled != nil {
		t.Errorf("Mismatching subscriber[0] Enabled, should be nil.\n")
	}
	if len(subslist.SubscriptionList[six_0].SoftwareStatus) != 0 {
		t.Errorf("Mismatching subscriber[0] SoftwareStatus, should be empty.\n")
	}
	if len(subslist.SubscriptionList[six_0].Roles) != 0 {
		t.Errorf("Mismatching subscriber[0] Roles, should be empty.\n")
	}

	//

	if subslist.SubscriptionList[six_1].Subscriber != "handler@x100c0s0b0n0" {
		t.Errorf("Mismatching subscriber: expecting 'handler@x100c0s0b0n0', got '%s'\n",
			subslist.SubscriptionList[six_1].Subscriber)
	}
	for ix, cc := range subdata2.ScnNodes {
		if subslist.SubscriptionList[six_1].Components[ix] != cc {
			t.Errorf("Mismatching subscriber[1] component[%d], expecting '%s', got '%s'\n",
				ix, cc, subslist.SubscriptionList[six_1].Components[ix])
		}
	}
	if subslist.SubscriptionList[six_1].States[0] != "off" {
		t.Errorf("Mismatching subscriber[1] state 0, expecting 'off', got '%s'\n",
			subslist.SubscriptionList[six_1].States[0])
	}
	if subslist.SubscriptionList[six_1].Url != "e.f.g.h" {
		t.Errorf("Mismatching subscriber[1] URL, expectin 'e.f.g.h', got '%s'\n",
			subslist.SubscriptionList[six_1].Url)
	}
	if subslist.SubscriptionList[six_1].Enabled == nil {
		t.Errorf("Mismatching subscriber[1] Enabled, should not be nil.\n")
	}
	if *subslist.SubscriptionList[six_1].Enabled != true {
		t.Errorf("Mismatching subscriber[1] Enabled, should be true.\n")
	}
	if len(subslist.SubscriptionList[six_1].SoftwareStatus) != 1 {
		t.Errorf("Mismatching subscriber[1] SoftwareStatus, should have exactly one entry, has %d.\n",
			len(subslist.SubscriptionList[six_1].SoftwareStatus))
	}
	if subslist.SubscriptionList[six_1].SoftwareStatus[0] != "admindown" {
		t.Errorf("Mismatching subscriber[1] SoftwareStatus, expecting 'admindown', got '%s'\n",
			subslist.SubscriptionList[six_1].SoftwareStatus[0])
	}
	if len(subslist.SubscriptionList[six_1].Roles) != 1 {
		t.Errorf("Mismatching subscriber[1] Roles, should have exactly one entry, has %d.\n",
			len(subslist.SubscriptionList[six_1].Roles))
	}
	if subslist.SubscriptionList[six_1].Roles[0] != "compute" {
		t.Errorf("Mismatching subscriber[1] Roles, expecting 'compute', got '%s'\n",
			subslist.SubscriptionList[six_1].Roles[0])
	}
	if subslist.SubscriptionList[six_1].SubRoles[0] != "ncn-m" {
		t.Errorf("Mismatching subscriber[1] SubRole 0, expecting 'ncn-m', got '%s'\n",
			subslist.SubscriptionList[six_1].SubRoles[0])
	}
	if subslist.SubscriptionList[six_1].SubRoles[1] != "ncn-w" {
		t.Errorf("Mismatching subscriber[1] SubRole 1, expecting 'ncn-w', got '%s'\n",
			subslist.SubscriptionList[six_1].SubRoles[1])
	}

	//Check disallowed operations

	req2_payload := bytes.NewBufferString("")
	req2, err2 := http.NewRequest("DELETE", "http://localhost:8080/hmnfd/v1/scn",
		req2_payload)
	if err2 != nil {
		t.Fatal(err2)
	}
	rr2 := httptest.NewRecorder()
	handler2 := http.HandlerFunc(doSubscribeDelete)
	handler2.ServeHTTP(rr2, req2)
	if rr2.Code == http.StatusOK {
		t.Errorf("Disallowed DELETE operation didn't fail, got response code %v\n",
			rr2.Code)
	}

	req3_payload := bytes.NewBufferString("")
	req3, err3 := http.NewRequest("POST", "http://localhost:8080/hmnfd/v1/scn",
		req3_payload)
	if err3 != nil {
		t.Fatal(err3)
	}
	rr3 := httptest.NewRecorder()
	handler3 := http.HandlerFunc(doSubscribePost)
	handler3.ServeHTTP(rr3, req3)
	if rr3.Code == http.StatusOK {
		t.Errorf("Disallowed POST operation didn't fail, got response code %v\n",
			rr3.Code)
	}

	req4_payload := bytes.NewBufferString("")
	req4, err4 := http.NewRequest("PATCH", "http://localhost:8080/hmnfd/v1/scn",
		req4_payload)
	if err4 != nil {
		t.Fatal(err4)
	}
	rr4 := httptest.NewRecorder()
	handler4 := http.HandlerFunc(doSubscribePatch)
	handler4.ServeHTTP(rr4, req4)
	if rr4.Code == http.StatusOK {
		t.Errorf("Disallowed PATCH operation didn't fail, got response code %v\n",
			rr4.Code)
	}
}

func TestPrune(t *testing.T) {
	var kverr error
	var subdata, subdata2, subdata3 SubData
	var kok bool
	var hmscn Scn

	disable_logs()

	//Make sure the pruning loop is running.
	go prune()
	if !gofuncsRunning {
		go handleSCNs()
		go checkSCNCache()
	}

	//Shortcut: stuff the ETCD KV with subscriptions, then use the func to
	//read them out.

	kvHandle, kverr = hmetcd.Open("mem:", "")
	if kverr != nil {
		t.Fatal("KV/ETCD open failed:", kverr)
	}
	defer kvPurge(t)

	subdata.Url = "a.b.c.d"
	subdata.ScnNodes = []string{"x1c1s1b0n1"}
	ba, baerr := json.Marshal(subdata)
	if baerr != nil {
		t.Fatal("Error marshalling JSON.")
	}

	subdata2.Url = "e.f.g.h"
	subdata2.ScnNodes = []string{"x2c2s2b0n2"}
	ba2, baerr2 := json.Marshal(subdata2)
	if baerr2 != nil {
		t.Fatal("Error marshalling JSON.")
	}

	subdata3.Url = "i.j.k.l"
	subdata3.ScnNodes = []string{"x3c3s3b0n3"}
	ba3, baerr3 := json.Marshal(subdata2)
	if baerr3 != nil {
		t.Fatal("Error marshalling JSON.")
	}

	key1 := "sub#x0c0s0b0n0#hs.ready#svc.foo"
	kverr = kvHandle.Store(key1, string(ba))
	if kverr != nil {
		t.Fatal("ERROR storing KV key in ETCD store.")
	}
	key2 := "sub#x100c0s0b0n0#hs.off#svc.bar"
	kverr = kvHandle.Store(key2, string(ba2))
	if kverr != nil {
		t.Fatal("ERROR storing KV key in ETCD store.")
	}
	key3 := "sub#x0c0s0b0n0#hs.on#svc.bazz"
	kverr = kvHandle.Store(key3, string(ba3))
	if kverr != nil {
		t.Fatal("ERROR storing KV key in ETCD store.")
	}

	//Do a DELETE of one of the subscriptions.  Verify that the other 2 are
	//still present and are not pruned.

	bs := "{\"Subscriber\":\"bar@x100c0s0b0n0\",\"Url\":\"e.f.g.h\"}"
	req_payload := bytes.NewBufferString(bs)
	req, err := http.NewRequest("DELETE", "http://localhost:8080/hmnfd/v1/subscribe",
		req_payload)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(doSubscribeDelete)

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("HTTP handler 'doSubscribeDelete' returned bad error code, got %v, want %v\n",
			rr.Code, http.StatusOK)
	}

	//Check the KV keys to be sure this one is gone but the others are there.

	_, kok, kverr = kvHandle.Get(key1)
	if !kok {
		t.Fatal("KV record", key1, "deleted, should not have been")
	}
	_, kok, kverr = kvHandle.Get(key2)
	if kok {
		t.Fatal("KV record", key2, "not deleted!")
	}
	_, kok, kverr = kvHandle.Get(key3)
	if !kok {
		t.Fatal("KV record", key3, "deleted, should not have been.")
	}

	//Do a prune by getting an SCN for OFF state which should delete all
	//subscriptions to the target node(s) in the SCN.  Verify that all
	//remaining subscriptions for the target OFF node are pruned.

	hmscn.Components = []string{"x0c0s0b0n0"}
	hmscn.State = "Off"
	hba, hbaerr := json.Marshal(hmscn)
	if hbaerr != nil {
		t.Fatal("ERROR marshaling JSON data!")
	}

	req2_payload := bytes.NewBuffer(hba)
	req2, err2 := http.NewRequest("POST", "http://localhost:8080/hmnfd/v1/scn",
		req2_payload)
	if err2 != nil {
		t.Fatal(err2)
	}

	rr2 := httptest.NewRecorder()
	handler2 := http.HandlerFunc(scnHandler)
	handler2.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("POST operation failed, got response bode %v\n", rr2.Code)
	}

	time.Sleep(12 * time.Second) //the prune loop looks every 10 seconds.

	//Make sure the other 2 subscriptions, all based on x0c0s0b0n0, are gone.

	_, kok, kverr = kvHandle.Get(key1)
	if kok {
		t.Error("KV record", key1, "not deleted!")
	}
	_, kok, kverr = kvHandle.Get(key3)
	if kok {
		t.Error("KV record", key3, "not deleted!")
	}
}

func TestSendToTelemetryBus(t *testing.T) {
	var scn, jdata Scn
	bt := true

	app_params.Use_telemetry = 1
	scn.Components = []string{"x0c1s2b3n4"}
	scn.Enabled = &bt
	scn.Flag = "OK"
	scn.Role = "Compute"
	scn.SubRole = "NCN"
	scn.SoftwareStatus = "READY"
	scn.State = "Ready"
	scn.Timestamp = "01-02-2021T01:02:03"

	sendToTelemetryBus(scn)
	time.Sleep(time.Second)

	scnStr := <-kq_chan
	app_params.Use_telemetry = 0

	err := json.Unmarshal([]byte(scnStr), &jdata)
	if err != nil {
		t.Errorf("ERROR unmarshalling SCN: %v", err)
	}
	if len(jdata.Components) != len(scn.Components) {
		t.Errorf("ERROR, num components mismatch: exp: %d, got: %d",
			len(scn.Components), len(jdata.Components))
	}
	if jdata.Components[0] != scn.Components[0] {
		t.Errorf("ERROR, component name mismatch, exp: '%s', got '%s'",
			scn.Components[0], jdata.Components[0])
	}
	if jdata.Enabled == nil {
		t.Errorf("ERROR, Enabled is nil.")
	}
	if *jdata.Enabled != true {
		t.Errorf("ERROR, Enabled is false.")
	}
	if jdata.Flag != scn.Flag {
		t.Errorf("ERROR, Flag mismatch, exp: '%s', got: '%s'",
			scn.Flag, jdata.Flag)
	}
	if jdata.Role != scn.Role {
		t.Errorf("ERROR, Role mismatch, exp: '%s', got: '%s'",
			scn.Role, jdata.Role)
	}
	if jdata.SubRole != scn.SubRole {
		t.Errorf("ERROR, RoleFlag mismatch, exp: '%s', got: '%s'",
			scn.SubRole, jdata.SubRole)
	}
	if jdata.SoftwareStatus != scn.SoftwareStatus {
		t.Errorf("ERROR, SoftwareStatus mismatch, exp: '%s', got: '%s'",
			scn.SoftwareStatus, jdata.SoftwareStatus)
	}
	if jdata.State != scn.State {
		t.Errorf("ERROR, State mismatch, exp: '%s', got: '%s'",
			scn.State, jdata.State)
	}
	if jdata.Timestamp != scn.Timestamp {
		t.Errorf("ERROR, Timestamp mismatch, exp: '%s', got: '%s'",
			scn.Timestamp, jdata.Timestamp)
	}
}
