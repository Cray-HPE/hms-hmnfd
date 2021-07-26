// MIT License
//
// (C) Copyright [2020-2021] Hewlett Packard Enterprise Development LP
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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Cray-HPE/hms-hmetcd"
)

func TestLiveness(t *testing.T) {
	// intialize a bunch of stuff for the tests
	hstuff := new(httpStuff)
	var ba []byte
	reqPayload := bytes.NewBuffer(ba)
	handler1 := http.HandlerFunc(hstuff.livenessHandler)

	// test valid request
	req1, _ := http.NewRequest("GET", "http://localhost:8080/hmnfd/v1/liveness", reqPayload)
	rr1 := httptest.NewRecorder()
	handler1.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusNoContent {
		t.Errorf("GET operation failed, got response code %v\n", rr1.Code)
	}

	// test invalid request
	req2, _ := http.NewRequest("PUT", "http://localhost:8080/hmnfd/v1/liveness", reqPayload)
	rr2 := httptest.NewRecorder()
	handler1.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT operation failed, got response code %v\n", rr2.Code)
	}

	// test invalid request
	req3, _ := http.NewRequest("POST", "http://localhost:8080/hmnfd/v1/liveness", reqPayload)
	rr3 := httptest.NewRecorder()
	handler1.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST operation failed, got response code %v\n", rr3.Code)
	}
}

func TestReadiness(t *testing.T) {
	// intialize a bunch of stuff for the tests
	hstuff := new(httpStuff)
	var ba []byte
	reqPayload := bytes.NewBuffer(ba)
	handler1 := http.HandlerFunc(hstuff.readinessHandler)

	// test valid request
	req1, _ := http.NewRequest("GET", "http://localhost:8080/hmnfd/v1/readiness", reqPayload)
	rr1 := httptest.NewRecorder()
	handler1.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusServiceUnavailable {
		t.Errorf("GET operation expected service unavailable, got response code %v\n", rr1.Code)
	}

	// test invalid request
	req2, _ := http.NewRequest("PUT", "http://localhost:8080/hmnfd/v1/readiness", reqPayload)
	rr2 := httptest.NewRecorder()
	handler1.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT operation failed, got response code %v\n", rr2.Code)
	}

	// test invalid request
	req3, _ := http.NewRequest("POST", "http://localhost:8080/hmnfd/v1/readiness", reqPayload)
	rr3 := httptest.NewRecorder()
	handler1.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST operation failed, got response code %v\n", rr3.Code)
	}

	// spin up enough things that readiness returns true
	// take existing KVStore if present and set aside - make sure
	// the existing one gets restored on exit
	pickledKV := kvHandle
	kvHandle = nil
	defer func() { kvHandle = pickledKV }()

	// start the KV Store and test again
	var kverr error
	kvHandle, kverr = hmetcd.Open("mem:", "")
	if kverr != nil {
		t.Fatal("KV/ETCD open failed:", kverr)
	}
	defer kvPurge(t)
	kvHandle.Store("HMNFD_HEALTH_KEY", "HMNFD_OK")

	req4, _ := http.NewRequest("GET", "http://localhost:8080/hmnfd/v1/readiness", reqPayload)
	rr4 := httptest.NewRecorder()
	handler1.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusNoContent {
		t.Errorf("GET operation expected success, got response code %v\n", rr4.Code)
	}

}

func TestHealth(t *testing.T) {
	// intialize a bunch of stuff for the tests
	hstuff := new(httpStuff)
	var ba []byte
	reqPayload := bytes.NewBuffer(ba)
	handler1 := http.HandlerFunc(hstuff.healthHandler)

	// take existing KVStore if present and set aside - make sure
	// the existing one gets restored on exit
	pickledKV := kvHandle
	kvHandle = nil
	defer func() { kvHandle = pickledKV }()

	// test valid request - KV Store not present
	req1, _ := http.NewRequest("GET", "http://localhost:8080/hmnfd/v1/health", reqPayload)
	rr1 := httptest.NewRecorder()
	handler1.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("GET operation failed, got response code %v\n", rr1.Code)
	}
	body, err := ioutil.ReadAll(rr1.Body)
	if err != nil {
		t.Fatal("ERROR reading GET response body:", err)
	}
	var stats HealthResponse
	err = json.Unmarshal(body, &stats)
	if err != nil {
		t.Fatal("ERROR unmarshalling GET response body:", err)
	}
	if stats.KvStoreStatus != "KV Store not initialized" {
		t.Fatal("Expected KV Store not initialized")
	}

	// test invalid request
	req2, _ := http.NewRequest("PUT", "http://localhost:8080/hmnfd/v1/health", reqPayload)
	rr2 := httptest.NewRecorder()
	handler1.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT operation failed, got response code %v\n", rr2.Code)
	}

	// test invalid request
	req3, _ := http.NewRequest("POST", "http://localhost:8080/hmnfd/v1/health", reqPayload)
	rr3 := httptest.NewRecorder()
	handler1.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST operation failed, got response code %v\n", rr3.Code)
	}

	// start the KV Store and test again
	var kverr error
	kvHandle, kverr = hmetcd.Open("mem:", "")
	if kverr != nil {
		t.Fatal("KV/ETCD open failed:", kverr)
	}
	defer kvPurge(t)
	kvHandle.Store("HMNFD_HEALTH_KEY", "HMNFD_OK")

	// send the request
	req4, _ := http.NewRequest("GET", "http://localhost:8080/hmnfd/v1/health", reqPayload)
	rr4 := httptest.NewRecorder()
	handler1.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("GET operation failed, got response code %v\n", rr4.Code)
	}
	body, err = ioutil.ReadAll(rr4.Body)
	if err != nil {
		t.Fatal("ERROR reading GET response body:", err)
	}
	var stats4 HealthResponse
	err = json.Unmarshal(body, &stats4)
	if err != nil {
		t.Fatal("ERROR unmarshalling GET response body:", err)
	}
	if stats4.KvStoreStatus != "Health key value:HMNFD_OK" {
		t.Fatal("Expected KV Store present with no key")
	}

}
