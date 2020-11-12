// Copyright (c) 2019 Cray Inc. All Rights Reserved.

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

