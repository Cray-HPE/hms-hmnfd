// MIT License
//
// (C) Copyright [2021] Hewlett Packard Enterprise Development LP
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Cray-HPE/hms-hmetcd"
)

type subStuff struct {
	url    string
	method string
}

func TestScnSubscribeHandler_v2(t *testing.T) {
	var kverr error
	var kvalue string
	var kok bool
	var kvval SubData

	disable_logs()

	routes := generateRoutes()
	router := newRouter(routes)

	//Set up ETCD

	kvHandle, kverr = hmetcd.Open("mem:", "")
	if kverr != nil {
		t.Fatal("KV/ETCD open failed:", kverr)
	}
	defer kvPurge(t)

	//Create 3 fake HTTP posts, one each for POST, PATCH, DELETE

	subkey1 := "sub#x0c1s2b0n3#hs.ready.standby#ss.admindown#enbl.enbl#roles.compute#subroles.ncn-m.ncn-w#svc.handler"

	comp1 := "x1000c2s3b0n4"
	comp2 := "x1000c2s3b0n5"
	state1 := "Ready"
	state2 := "Standby"
	url := "http://x0c1s2b0n3:8888/scn"
	bs1 := fmt.Sprintf("{\"Components\":[\"%s\",\"%s\"],\"States\":[\"%s\",\"%s\"],\"SoftwareStatus\":[\"AdminDown\"],\"Roles\":[\"Compute\"],\"SubRoles\":[\"ncn-m\",\"ncn-w\"],\"Enabled\":true,\"Url\":\"%s\"}",
		comp1, comp2, state1, state2, url)
	req1_payload := bytes.NewBufferString(bs1)
	req1, err1 := http.NewRequest("POST", "http://localhost:8080/hmi/v2/subscriptions/x0c1s2b0n3/agents/handler",
		req1_payload)
	if err1 != nil {
		t.Fatal(err1)
	}
	rr1 := httptest.NewRecorder()

	req3, err3 := http.NewRequest("DELETE", "http://localhost:8080/hmi/v2/subscriptions/x0c1s2b0n3/agents/handler", nil)
	if err3 != nil {
		t.Fatal(err3)
	}
	rr3 := httptest.NewRecorder()

	subkey4 := "sub#x0c1s2b0n3#hs.off#svc.handler"
	comp4_1 := "x2000c2s2b0n2"
	comp4_2 := "x3000c3s3b0n3"
	state4_1 := "Off"
	bs4 := fmt.Sprintf("{\"Components\":[\"%s\",\"%s\"],\"States\":[\"%s\"],\"Url\":\"%s\"}",
		comp4_1, comp4_2, state4_1, url)
	req4_payload := bytes.NewBufferString(bs4)
	req4, err4 := http.NewRequest("PATCH", "http://localhost:8080/hmi/v2/subscriptions/x0c1s2b0n3/agents/handler",
		req4_payload)
	if err4 != nil {
		t.Fatal(err4)
	}
	rr4 := httptest.NewRecorder()

	//Mock up the POST operation

	router.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("HTTP handler returned bad error code, got %v, want %v\n",
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

	router.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusNoContent {
		t.Errorf("HTTP handler returned bad error code, got %v, want %v\n",
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

	router.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusNoContent {
		t.Errorf("HTTP handler returned bad error code, got %v, want %v\n",
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

func TestSubscriptionsHandler_v2(t *testing.T) {
	var key string
	var kverr error
	var subslist SubscriptionList
	var subdata, subdata2 SubData

	disable_logs()

	routes := generateRoutes()
	router := newRouter(routes)

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

	key = "sub#x0c0s0b0n0#hs.ready.standby#svc.tube_processor"
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
	req1, err1 := http.NewRequest("GET", "http://localhost:8080/hmi/v2/subscriptions",
		req1_payload)
	if err1 != nil {
		t.Fatal(err1)
	}
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
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

	if subslist.SubscriptionList[six_0].SubscriberComponent != "x0c0s0b0n0" {
		t.Errorf("Mismatching subscriber: expecting 'x0c0s0b0n0', got '%s'\n",
			subslist.SubscriptionList[six_0].SubscriberComponent)
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

	if subslist.SubscriptionList[six_1].SubscriberComponent != "x100c0s0b0n0" {
		t.Errorf("Mismatching subscriber component: expecting 'x100c0s0b0n0', got '%s'\n",
			subslist.SubscriptionList[six_1].SubscriberComponent)
	}
	if subslist.SubscriptionList[six_1].SubscriberAgent != "handler" {
		t.Errorf("Mismatching subscriber agent: expecting 'handler', got '%s'\n",
			subslist.SubscriptionList[six_1].SubscriberAgent)
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

	//test bad requests

	ops := []subStuff{
		{url: "http://localhost:8080/hmi/v2/subscriptions", method: "PUT"},
		{url: "http://localhost:8080/hmi/v2/subscriptions/x0c0s0b0", method: "PUT"},
		{url: "http://localhost:8080/hmi/v2/subscriptions/x0c0s0b0/agents", method: "PUT"},
		{url: "http://localhost:8080/hmi/v2/subscriptions/x0c0s0b0/agents/handler", method: "PUT"},
		{url: "http://localhost:8080/hmi/v2/subscriptions/x0c0s0b0/agents", method: "GET"},
		{url: "http://localhost:8080/hmi/v2/subscriptions", method: "DELETE"},
		{url: "http://localhost:8080/hmi/v2/subscriptions/x0c0s0b0", method: "DELETE"},
		{url: "http://localhost:8080/hmi/v2/subscriptions", method: "PATCH"},
		{url: "http://localhost:8080/hmi/v2/subscriptions/x0c0s0b0", method: "PATCH"},
		{url: "http://localhost:8080/hmi/v2/subscriptions/x0c0s0b0/agents", method: "PATCH"},
	}

	req1_payload = bytes.NewBufferString("")
	for _, op := range ops {
		req1, err1 = http.NewRequest(op.method, op.url, req1_payload)
		if err1 != nil {
			t.Errorf("Error creating HTTP request: %v", err1)
		}
		rr1 = httptest.NewRecorder()
		router.ServeHTTP(rr1, req1)
		if rr1.Code == http.StatusOK {
			t.Errorf("ERROR, %s request for '%s' should have failed!",
				op.method, op.url)
		}
	}
}
