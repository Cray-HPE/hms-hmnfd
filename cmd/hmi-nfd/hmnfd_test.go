// Copyright (c) 2019 Cray Inc. All Rights Reserved.

package main

import (
    "testing"
    "os"
    "log"
    "io/ioutil"
)

type ppj struct {
    name string
    raw []byte
    errstr string
}

var param_exp = `{"Debug":1,"KV_url":"a.b.c.d","Nosm":1,"Port":1234,"Scn_in_url":"e.f.g.h","Scn_max_cache":56,"Scn_cache_delay":78,"SM_retries":12,"SM_timeout":34,"SM_url":"e.f.g.h","Telemetry_host":"aaaa:1234:bbbb","Use_telemetry":0}`

var param_inp_patch = `{"Debug":1,"KV_url":"a.b.c.d","Nosm":1,"SM_retries":12,"SM_timeout":34,"SM_url":"e.f.g.h","Scn_max_cache":56,"Scn_cache_delay":78}`


func disable_logs() {
    log.SetFlags(0)
    log.SetOutput(ioutil.Discard)
}

func enable_logs() {
    log.SetOutput(os.Stdout)
}

func TestGenCurParamJson(t *testing.T) {
    disable_logs()
    app_params.Debug = 1
    app_params.Help = 0
    app_params.KV_url = "a.b.c.d"
    app_params.Nosm = 1
    app_params.Port = 1234
    app_params.Scn_in_url = "e.f.g.h"
    app_params.Scn_max_cache = 56
    app_params.Scn_cache_delay = 78
    app_params.SM_retries = 12
    app_params.SM_timeout = 34
    app_params.SM_url = "e.f.g.h"
    app_params.Telemetry_host = "aaaa:1234:bbbb"
    app_params.Use_telemetry = 0

    ba,err := genCurParamJson()
    if (err != nil) {
        t.Error("genCurParamJson() failed:",err)
    }
    if (string(ba) != param_exp) {
        t.Errorf("Miscompare of genCurParamJson() output: exp: '%s', got: '%s'\n",
            param_exp,string(ba))
    }
}

func TestParseCmdLine(t *testing.T) {
    disable_logs()
    app_params = opParams{} //reset to all 0

    os.Args = []string{"app", "--debug=1", "--kv_url=a.b.c.d", "--nosm",
                       "--port=1234", "--scn_in_url=e.f.g.h",
                       "--scn_max_cache=56", "--scn_cache_delay=78",
                       "--sm_retries=12", "--sm_timeout=34",
                       "--sm_url=e.f.g.h", "--telemetry_host=aaaa:1234:bbbb",
                       "--use_telemetry=0"}

    parseCmdLine()
    ba,err := genCurParamJson()
    if (err != nil) {
        t.Error("genCurParamJson() failed:",err)
    }
    if (string(ba) != param_exp) {
        t.Errorf("Miscompare of genCurParamJson() output: exp: '%s', got: '%s'\n",
            param_exp,string(ba))
    }
}

func TestParseEnvVars(t *testing.T) {
    disable_logs()
    app_params = opParams{} //reset to all 0

    os.Setenv("HMNFD_DEBUG","1")
    os.Setenv("HMNFD_KV_URL","a.b.c.d")
    os.Setenv("HMNFD_NOSM","1")
    os.Setenv("HMNFD_PORT","1234")
    os.Setenv("HMNFD_SCN_IN_URL","e.f.g.h")
    os.Setenv("HMNFD_SCN_MAX_CACHE","56")
    os.Setenv("HMNFD_SCN_CACHE_DELAY","78")
    os.Setenv("HMNFD_SM_RETRIES","12")
    os.Setenv("HMNFD_SM_TIMEOUT","34")
    os.Setenv("HMNFD_SM_URL","e.f.g.h")
    os.Setenv("HMNFD_TELEMETRY_HOST","aaaa:1234:bbbb")
    os.Setenv("HMNFD_USE_TELEMETRY","0")

    parseEnvVars()
    ba,err := genCurParamJson()
    if (err != nil) {
        t.Error("genCurParamJson() failed:",err)
    }
    if (string(ba) != param_exp) {
        t.Errorf("Miscompare of genCurParamJson() output: exp: '%s', got: '%s'\n",
            param_exp,string(ba))
    }
}

func TestParseParamJson(t *testing.T) {
    app_params = opParams{} //reset to all 0
    var ba []byte
    var err error

    disable_logs()

    //Test normal operation from startup context

    err = parseParamJson([]byte(param_exp), PARAM_START)
    if (err != nil) {
        t.Error("parseParamJson() failed.")
    }

    ba,err = genCurParamJson()
    if (err != nil) {
        t.Error("genCurParamJson() failed:",err)
    }
    if (string(ba) != param_exp) {
        t.Errorf("Miscompare of genCurParamJson(START) output: exp: '%s', got: '%s'\n",
            param_exp,string(ba))
    }

    //Test normal operation from PATCH context

    app_params = opParams{} //reset to all 0
    app_params.Debug = 1
    app_params.KV_url = "a.b.c.d"
    app_params.Nosm = 1
    app_params.Port = 1234
    app_params.Scn_in_url = "e.f.g.h"
    app_params.Scn_max_cache = 56
    app_params.Scn_cache_delay = 78
    app_params.SM_retries = 12
    app_params.SM_timeout = 34
    app_params.SM_url = "e.f.g.h"
    app_params.Telemetry_host = "aaaa:1234:bbbb"
    app_params.Use_telemetry = 0

    err = parseParamJson([]byte(param_inp_patch), PARAM_PATCH)
    if (err != nil) {
        t.Error("parseParamJson() failed.")
    }

    ba,err = genCurParamJson()
    if (err != nil) {
        t.Error("genCurParamJson() failed:",err)
    }
    if (string(ba) != param_exp) {
        t.Errorf("Miscompare of genCurParamJson(PATCH) output:\nexp: '%s'\ngot: '%s'\n",
            param_exp,string(ba))
    }

    //Test a bunch of incorrect data type errors

    var vectors = []ppj{ 
                    {name: "Debug",
                     raw: []byte("{\"Debug\":\"1\"}"),
                     errstr: "Invalid data type in Debug field. ",
                    },
                    {name: "Nosm",
                     raw: []byte("{\"Nosm\":\"1\"}"),
                     errstr: "Invalid data type in Nosm field. ",
                    },
                    {name: "Port",
                     raw: []byte("{\"Port\":\"1\"}"),
                     errstr: "Invalid data type in Port field. ",
                    },
                    {name: "KV_url",
                     raw: []byte("{\"KV_url\":1234}"),
                     errstr: "Invalid data type in KV_url field. ",
                    },
                    {name: "Scn_in_url",
                     raw: []byte("{\"Scn_in_url\":1234}"),
                     errstr: "Invalid data type in Scn_in_url field. ",
                    },
                    {name: "Scn_max_cache",
                     raw: []byte("{\"Scn_max_cache\":\"1234\"}"),
                     errstr: "Invalid data type in Scn_max_cache field. ",
                    },
                    {name: "Scn_cache_delay",
                     raw: []byte("{\"Scn_cache_delay\":\"1234\"}"),
                     errstr: "Invalid data type in Scn_cache_delay field. ",
                    },
                    {name: "SM_url",
                     raw: []byte("{\"SM_url\":1234}"),
                     errstr: "Invalid data type in SM_url field. ",
                    },
                    {name: "Telemetry_host",
                     raw: []byte("{\"Telemetry_host\":1234}"),
                     errstr: "Invalid data type in Telemetry_host field. ",
                    },
    }

    for _,vv := range vectors {
        err = parseParamJson(vv.raw,PARAM_START)
        if (err == nil) {
            t.Errorf("Unexpected pass of parseParamJson() with bad %s data type.",
                vv.name)
        }
        if (err.Error() != vv.errstr) {
            t.Errorf("Mismatch %s error string, expected: '%s', got: '%s'\n",
                vv.name,vv.errstr,err.Error())
        }
    }
}

