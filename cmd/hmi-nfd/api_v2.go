// MIT License
//
// (C) Copyright [2019-2021,2023] Hewlett Packard Enterprise Development LP
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
	"github.com/Cray-HPE/hms-base"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

func subscriptionsAgentDeleteHandler(w http.ResponseWriter, r *http.Request) {
	// /subscriptions/{xname}/agent/{agent}
	uvars := mux.Vars(r)
	xn, _ := uvars["xname"]
	agent, _ := uvars["agent"]
	xname := base.VerifyNormalizeCompID(xn)

	if xname == "" {
		log.Printf("ERROR: Invalid XName.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Invalid XName in URL path",
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	log.Printf("Received a subscription DELETE request.\n")

	skvlist, serr := kvHandle.GetRange(SUBSCRIBER_KEYRANGE_START, SUBSCRIBER_KEYRANGE_END)
	if serr != nil {
		log.Println("ERROR fetching subscription keys:", serr)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Key/Value ETCD service GET operation failed",
			r.URL.Path, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	matched := false

	for _, sub := range skvlist {
		//Get the service name from the key.  It can be a simple XName or svc@XName.
		//Gotta get the XName token (always first after sub), then get optional
		//service name (defined by svc.xxx in the key)

		toks := strings.Split(sub.Key, SUBSCRIBER_KEY_DELIM)
		subXName := toks[SUBSCRIBER_TOKNUM_XNAME]
		subAgent := ""
		subsvc := subXName
		ix := strings.Index(sub.Key, SUBSCRIBER_KEY_SVC)
		if ix > 0 {
			tt := strings.Split(sub.Key[(ix+len(SUBSCRIBER_KEY_SVC)+1):], SUBSCRIBER_KEY_DELIM)
			subAgent = tt[0]
			subsvc = tt[0] + SUBSCRIBER_SVC_DELIM + subXName
		}

		if app_params.Debug > 1 {
			log.Printf("Found subscriber info: '%s' @ '%s'\n", subXName, subAgent)
		}
		if (xname == subXName) && (agent == subAgent) {
			matched = true
			if app_params.Debug > 1 {
				log.Printf("MATCHED subscription key for deletion: '%s' @ '%s'\n",
					xname, agent)
			}
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

	if !matched {
		log.Printf("ERROR no matching subscription for DELETE '%s' @ '%s':",
			xname, agent)
		pdet := base.NewProblemDetails("about:blank",
			"Bad DELETE request",
			"No matching subscription for DELETE",
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func subscriptionsXNameDeleteHandler(w http.ResponseWriter, r *http.Request) {
	// /subscriptions/{xname}/agent
	uvars := mux.Vars(r)
	xn, _ := uvars["xname"]
	xname := base.VerifyNormalizeCompID(xn)

	if xname == "" {
		log.Printf("ERROR: Invalid XName.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Invalid XName in URL path",
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	log.Printf("Received a subscription DELETE request.\n")

	skvlist, serr := kvHandle.GetRange(SUBSCRIBER_KEYRANGE_START, SUBSCRIBER_KEYRANGE_END)
	if serr != nil {
		log.Println("ERROR fetching subscription keys:", serr)
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			"Key/Value ETCD service GET operation failed",
			r.URL.Path, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	for _, sub := range skvlist {
		//Get the service name from the key.  It can be a simple XName or svc@XName.
		//Gotta get the XName token (always first after sub), then get optional
		//service name (defined by svc.xxx in the key)

		toks := strings.Split(sub.Key, SUBSCRIBER_KEY_DELIM)
		subXName := toks[SUBSCRIBER_TOKNUM_XNAME]
		subAgent := ""
		subsvc := subXName
		ix := strings.Index(sub.Key, SUBSCRIBER_KEY_SVC)
		if ix > 0 {
			tt := strings.Split(sub.Key[(ix+len(SUBSCRIBER_KEY_SVC)+1):], SUBSCRIBER_KEY_DELIM)
			subAgent = tt[0]
			subsvc = tt[0] + SUBSCRIBER_SVC_DELIM + subXName
		}

		if app_params.Debug > 1 {
			log.Printf("Found subscriber info: '%s' @ '%s'\n", subXName, subAgent)
		}
		if xname == subXName {
			if app_params.Debug > 1 {
				log.Printf("MATCHED subscription key for deletion: '%s' @ '%s'\n",
					subXName, subAgent)
			}
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
	w.WriteHeader(http.StatusNoContent)
}

func subscriptionsAgentPostHandler(w http.ResponseWriter, r *http.Request) {
	var jdata ScnSubscribe

	// /subscriptions/{xname}/agents/{agent}
	uvars := mux.Vars(r)
	xn, _ := uvars["xname"]
	agent, _ := uvars["agent"]
	xname := base.VerifyNormalizeCompID(xn)

	if xname == "" {
		log.Printf("ERROR: Invalid XName.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Invalid XName in URL path",
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error on message read:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Error reading inbound request body",
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	err = json.Unmarshal([]byte(strings.ToLower(string(body))), &jdata)
	if err != nil {
		handleSubscribePostError(r.URL.Path, w, body)
		return
	}

	//Make sure all mandatory fields are present.

	err = checkSubscription_v2(jdata)
	if err != nil {
		log.Println("Missing subscription payload fields:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			err.Error(),
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	if app_params.Debug > 0 {
		log.Printf("Received a subscription POST request, payload '%s'.\n",
			string(body))
	}

	if base.GetHMSType(xname) == base.HMSTypeInvalid {
		//This is not a valid XName.  We can't accept this, since it will
		//make pruning not work.
		log.Printf("Invalid subscriber XName: '%s'.\n", xname)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Subcriber is not a valid XName",
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	//Construct the subscription ETCD key and see if it already exists.

	subKey := makeSubscriptionKey_V2(jdata, xname, agent)

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
			r.URL.Path, http.StatusInternalServerError)
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
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	//No existing key.  Make one.

	err = makeSubscriptionEntry(jdata.Components, jdata.Url, subKey)
	if err != nil {
		pdet := base.NewProblemDetails("about:blank",
			"Internal Server Error",
			err.Error(),
			r.URL.Path, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}
	hsmsub_chan <- jdata //subscribe to SCN from HSM

	w.Header().Add("Connection", "close")
	w.WriteHeader(http.StatusOK)
}

/////////////////////////////////////////////////////////////////////////////
// Does the dirty work of a PATCH operation on the
// /subscriptions/{xname}/agents/{agent} API endpoint.
//
// w(in):  HTTP response writer.
// r(in):  HTTP request.
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func subscriptionsAgentPatchHandler(w http.ResponseWriter, r *http.Request) {
	var jdata ScnSubscribe

	// /subscriptions/{xname}/agents/{agent}
	uvars := mux.Vars(r)
	xn, _ := uvars["xname"]
	agent, _ := uvars["agent"]
	xname := base.VerifyNormalizeCompID(xn)

	if xname == "" {
		log.Printf("ERROR: Invalid XName.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Invalid XName in URL path",
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error on message read:", err)
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Error reading inbound request body",
			r.URL.Path, http.StatusBadRequest)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	err = json.Unmarshal([]byte(strings.ToLower(string(body))), &jdata)
	if err != nil {
		handleSubscribePostError(r.URL.Path, w, body)
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
			r.URL.Path, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	svcKeyDesc := SUBSCRIBER_KEY_SVC + SUBSCRIBER_KEYCAT_DELIM

	for _, kv := range skvlist {
		toks := strings.Split(kv.Key, SUBSCRIBER_KEY_DELIM)
		svcXName := toks[SUBSCRIBER_TOKNUM_XNAME]
		svcAgent := ""

		//Find the service token, if any, match with jdata.Subscriber

		ix := strings.Index(kv.Key, svcKeyDesc)
		if ix > 0 {
			iy := strings.Index(kv.Key[ix:], SUBSCRIBER_KEY_DELIM)
			if iy == -1 {
				svcAgent = kv.Key[ix+len(svcKeyDesc):]
			} else {
				svcAgent = kv.Key[ix+len(svcKeyDesc) : iy-1]
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
				r.URL.Path, http.StatusInternalServerError)
			base.SendProblemDetails(w, pdet, 0)
			return
		}

		if (svcXName == xname) && (svcAgent == agent) {
			//Match!  See if this subscription is an exact match. If so,
			//just replace the contents (key val).  If not, delete this key
			//and make a new one.

			var newSD SubData
			newSD.Url = jdata.Url
			newSD.ScnNodes = jdata.Components

			exKey := makeSubscriptionKey_V2(jdata, xname, agent)
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
						r.URL.Path, http.StatusInternalServerError)
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

	w.WriteHeader(http.StatusNoContent)
}

/////////////////////////////////////////////////////////////////////////////
// Get a list of current SCN subscriptions for a component.
// Returns JSON payload of all current subscription data to the caller.
//
// w(in):  HTTP response writer
// r(in):  HTTP request
// Return: None.
/////////////////////////////////////////////////////////////////////////////

func subscriptionsXNameGetHandler(w http.ResponseWriter, r *http.Request) {
	var sublist SubscriptionList

	if r.Method != "GET" {
		log.Printf("ERROR: request is not a GET.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Only GET operations supported",
			r.URL.Path, http.StatusMethodNotAllowed)
		//It is required to have an "Allow:" header with this error
		w.Header().Add("Allow", "GET")
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	uvars := mux.Vars(r)
	xnraw, _ := uvars["xname"]
	xname := base.VerifyNormalizeCompID(xnraw)
	if xname == "" {
		log.Printf("ERROR: Invalid XName.\n")
		pdet := base.NewProblemDetails("about:blank",
			"Invalid Request",
			"Invalid XName in URL path",
			r.URL.Path, http.StatusBadRequest)
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
			r.URL.Path, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	for _, item := range kvlist {
		var subinfo ScnSubscribe
		var subkeydata SubData

		//Separate the fields of the key to populate a subscription record.
		//Match up with the URL XName component.

		toks := strings.Split(item.Key, SUBSCRIBER_KEY_DELIM)
		if toks[SUBSCRIBER_TOKNUM_XNAME] != xname {
			continue
		}

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
				r.URL.Path, http.StatusInternalServerError)
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
			r.URL.Path, http.StatusInternalServerError)
		base.SendProblemDetails(w, pdet, 0)
		return
	}

	//Return the JSON payload
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(ba)
}
