// MIT License
//
// (C) Copyright [2019, 2021] Hewlett Packard Enterprise Development LP
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

import(
    "testing"
    "time"
    "stash.us.cray.com/HMS/hms-hmetcd"
    "encoding/json"
)


func saContains(sa []string, comp string) bool {
    for _,item  := range sa {
        if (item == comp) {
            return true
        }
    }
    return false
}

//Note that most of fanout.go is tested by other stuff.  This test doesn't
//do anything much of any value other than cover lines of code.

func TestSubscribeToHsmScn(t *testing.T) {
    var subdata ScnSubscribe
    var hsmsubinfo,hkv hsmSubscriptionInfo

    disable_logs()
    app_params.Nosm = 1 //don't actually send anything to SM!

    //connect the KV store, purge, reopen.  This clears the slate
    //for our testing.

    kvHandle,kvherr := hmetcd.Open("mem:","")
    if (kvherr != nil) {
        t.Fatal("KV/ETCD open failed:",kvherr)
    }
    kvPurge(t)
    kvHandle,kvherr = hmetcd.Open("mem:","")
    if (kvherr != nil) {
        t.Fatal("KV/ETCD open failed:",kvherr)
    }
    defer kvPurge(t)

    go subscribeToHsmScn()

    //Submit a subscription to the subscription chan.  Note that the KV store
    //is not initialized yet, so this will fail, and retry.

    enbl := true
    subdata.Subscriber = "handler@x1c2s3b0n4"
    subdata.Url = "http://a.b.c.d/scn"
    subdata.States = []string{"Ready","Standby"}
    subdata.Enabled = &enbl
    subdata.SoftwareStatus = []string{"AdminDown","AdminUp"}
    subdata.Roles = []string{"Compute","Service"}

    //This is what should end up in the ETCD key value

    hsmsubinfo.HWStates = []string{"ready","standby"}
    hsmsubinfo.SWStatus = []string{"admindown","adminup"}
    hsmsubinfo.Roles    = []string{"compute","service"}
    hsmsubinfo.Enabled  = true

    hsmsub_chan <- subdata
    time.Sleep(3 * time.Second)

    //Check the KV store for an artifact

    kv,kvok,kverr := kvHandle.Get(HSM_SUBS_KEY)
    if (kverr != nil) {
        t.Errorf("ERROR, failed to fetch key '%s': %s\n",HSM_SUBS_KEY,kverr.Error())
    }
    if (!kvok) {
        t.Errorf("ERROR, key '%s' not created!\n",HSM_SUBS_KEY)
    }

    //Unmarshal the key's value

    err := json.Unmarshal([]byte(kv),&hkv)
    if (err != nil) {
        t.Errorf("ERROR unmarshalling HSM subscription data '%s'.\n",kv)
    }

    //Compare.  Unfortunately we have to manually search, as ETCD key data
    //is not guaranteed in any particular order.

    if (len(hkv.HWStates) != 2) {
        t.Errorf("ERROR, subscription key HWStates should have 2 values, has %d\n",
            len(hkv.HWStates))
    }
    if (!saContains(hkv.HWStates,"ready")) {
        t.Errorf("ERROR, subscription key HWStates missing 'ready' entry.\n")
    }
    if (!saContains(hkv.HWStates,"standby")) {
        t.Errorf("ERROR, subscription key HWStates missing 'standby' entry.\n")
    }
    if (len(hkv.SWStatus) != 2) {
        t.Errorf("ERROR, subscription key SWStatus should have 2 values, has %d\n",
            len(hkv.SWStatus))
    }
    if (!saContains(hkv.SWStatus,"admindown")) {
        t.Errorf("ERROR, subscription key SWStatus missing 'admindown' entry.\n")
    }
    if (!saContains(hkv.SWStatus,"adminup")) {
        t.Errorf("ERROR, subscription key SWStatus missing 'adminup' entry.\n")
    }
    if (len(hkv.Roles) != 2) {
        t.Errorf("ERROR, subscription key Roles should have 2 values, has %d\n",
            len(hkv.Roles))
    }
    if (!saContains(hkv.Roles,"compute")) {
        t.Errorf("ERROR, subscription key Roles missing 'compute' entry.\n")
    }
    if (!saContains(hkv.Roles,"service")) {
        t.Errorf("ERROR, subscription key Roles missing 'service' entry.\n")
    }
    if (hkv.Enabled != true) {
        t.Errorf("ERROR, subscription key Enabled should be 'true', is 'false'.\n")
    }
}

