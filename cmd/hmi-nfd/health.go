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
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"stash.us.cray.com/HMS/hms-base"
)

// HealthResponse - used to report service health stats
type HealthResponse struct {
	KvStoreStatus         string `json:"KvStore"`
	MsgBusStatus          string `json:"MsgBus"`
	HsmSubscriptionStatus string `json:"HsmSubscriptions"`
	PruneMapStatus        string `json:"PruneMap"`
	WorkerPoolStatus      string `json:"WorkerPool"`
}

// doHealth - returns useful information about the service to the user
func (p *httpStuff) healthHandler(w http.ResponseWriter, r *http.Request) {
	// NOTE: this is provided as a debugging aid for administrators to
	//  find out what is going on with the system.  This should return
	//  information in a human-readable format that will help to
	//  determine the state of this service.

	// only allow 'GET' calls
	errinst := "/" + URL_HEALTH
	if r.Method != http.MethodGet {
		log.Printf("ERROR: request is not a GET.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Only GET operation supported",
			errinst, http.StatusMethodNotAllowed)
		//It is required to have an "Allow:" header with this error
		w.Header().Add("Allow", "GET")
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	// collect health information
	var stats HealthResponse

	// TODO - do we want to add config info here????

	// KV Store: openKV()
	// stored as part of init, should be able to query it if all is well:
	if kvHandle != nil {
		val, ok, kerr := kvHandle.Get("HMNFD_HEALTH_KEY")
		if kerr != nil {
			stats.KvStoreStatus = fmt.Sprintf("Error retrieving key:%s", kerr.Error())
		} else if !ok {
			stats.KvStoreStatus = "Health key not present"
		} else {
			stats.KvStoreStatus = fmt.Sprintf("Health key value:%s", val)
		}
	} else {
		stats.KvStoreStatus = "KV Store not initialized"
	}

	// Telemetry bus:  go telebusConnect()
	if msgbusHandle != nil {
		// NOTE: status==1 -> Open, 2 -> closed (msgbus.go defs of StatusOpen, StatusClosed)
		st := msgbusHandle.Status()
		if st == 1 {
			stats.MsgBusStatus = "Connected and OPEN"
		} else if st == 2 {
			stats.MsgBusStatus = "Connected and CLOSED"
		} else {
			stats.MsgBusStatus = fmt.Sprintf("Connected with unknown status:%d", st)
		}
	} else {
		stats.MsgBusStatus = "Not Connected"
	}

	// HSM subscriber thread: go subscribeToHsmScn()
	if kvHandle != nil {
		subVal, ok, serr := kvHandle.Get(HSM_SUBS_KEY)
		if serr != nil {
			stats.HsmSubscriptionStatus = fmt.Sprintf("HSM Subscription key retrieval error:%s", serr.Error())
		} else if !ok {
			stats.HsmSubscriptionStatus = "HSM Subscription key not present"
		} else {
			// TODO - too much?  Want summary instead?
			stats.HsmSubscriptionStatus = fmt.Sprintf("HSM Subscription: %s", subVal)
		}
	} else {
		stats.HsmSubscriptionStatus = "KVStore not initialized"
	}

	// subscription pruner: go prune()
	if len(prunemap) > 0 {
		stats.PruneMapStatus = fmt.Sprintf("Number of items:%d", len(prunemap))
	} else {
		stats.PruneMapStatus = "No contents"
	}

	// send telemetry requests: go telemetryBusSend()
	// Maybe log last send time / # requests sent for reading here?

	// worker pool: scnWorkPool
	// is there a way to see how many workers are present and how many jobs are queued?
	if scnWorkPool == nil {
		stats.WorkerPoolStatus = "Worker Pool not started"
	} else {
		stats.WorkerPoolStatus = fmt.Sprintf("Workers:%d, Jobs:%d", len(scnWorkPool.Workers),
			len(scnWorkPool.JobQueue))
	}

	// write the output
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
	return
}

// doReadiness - used for k8s readiness check
func (p *httpStuff) readinessHandler(w http.ResponseWriter, r *http.Request) {
	// NOTE: this is coded in accordance with kubernetes best practices
	//  for liveness/readiness checks.  This function should only be
	//  used to indicate if something is wrong with this service that
	//  prevents usage.  If this fails too many times, the instance
	//  will be killed and re-started.  Only fail this if restarting
	//  this service is likely to fix the problem.

	// only allow 'GET' calls
	errinst := "/" + URL_READINESS
	if r.Method != http.MethodGet {
		log.Printf("ERROR: request is not a GET.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Only GET operation supported",
			errinst, http.StatusMethodNotAllowed)
		//It is required to have an "Allow:" header with this error
		w.Header().Add("Allow", "GET")
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	// TODO - what dependent services should be checked that restarting may help?
	// Worker Pool - way to determine if stuck?

	ready := true

	// Message bus
	// NOTE: msgbusHangdle==nil is a valid state, don't fail for that
	// NOTE: status==1 -> Open, 2 -> closed (msgbus.go defs of StatusOpen, StatusClosed)
	if msgbusHandle != nil && msgbusHandle.Status() == 2 {
		// this is the case of bus created (used to work), but not working now
		log.Printf("ERROR: Readiness check message bus created but closed")
		ready = false
	}

	// stored as part of init, should be able to query it if all is well:
	if kvHandle != nil {
		// KV Started - see if all is well:
		_, ok, kerr := kvHandle.Get("HMNFD_HEALTH_KEY")
		if kerr != nil {
			log.Printf("ERROR: Readiness check KV Store 'Get' error:%s", kerr.Error())
			ready = false
		} else if !ok {
			log.Printf("ERROR: Readiness check KV Store failed to retrieve key 'HMNFD_HEALTH_KEY'")
			ready = false
		}
	} else {
		// KV Store not started
		log.Printf("ERROR: Readiness check KV Store not initialized")
		ready = false
	}

	// fail if anything determined not ready
	if ready {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	return
}

// doLiveness - used for k8s liveness check
func (p *httpStuff) livenessHandler(w http.ResponseWriter, r *http.Request) {
	// NOTE: this is coded in accordance with kubernetes best practices
	//  for liveness/readiness checks.  This function should only be
	//  used to indicate the server is still alive and processing requests.

	// only allow 'GET' calls
	errinst := "/" + URL_LIVENESS
	if r.Method != http.MethodGet {
		log.Printf("ERROR: request is not a GET.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Only GET operation supported",
			errinst, http.StatusMethodNotAllowed)
		//It is required to have an "Allow:" header with this error
		w.Header().Add("Allow", "GET")
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	// return simple StatusOK response to indicate server is alive
	w.WriteHeader(http.StatusNoContent)
	return
}
