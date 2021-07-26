# Cray State Change Notification Service (hmnfd)

The Shasta State Change Notification service provides the ability to 
notify subscribers of component hardware state changes and other changes
made to and by the Hardware State Manager.  _hmnfd_ is a companion service
to the Hardware State Manager in that it does the heavy lifting of 
distributing state changes and managing subscriptions.

This service uses a RESTful interface to provide the following functions:

* Subscription for component state changes:
  * Hardware state (On, Off, Ready, etc.)
  * Logical state (arbitrary, e.g., AdminDown)
  * Role (Compute, NCN, etc.)
  * Enabled state
  * Flag (OK, Alert, Warning, etc.)
* Ability to view current subscriptions
* Ability to delete subscriptions
* Ability to get and modify current service operating parameters
* Ability to receive State Change Notifications from the Hardware State Manager

_hmnfd_ typically runs on an SMS cluster as one or more Docker container 
instances managed by Kubernetes.  It can also be run from a command shell 
for testing purposes.

## hmnfd API

_hmnfd_'s RESTful API is a follows:

```bash
/v1/subscribe

    POST a subscription request for State Change Notifications.
```

```bash
/v1/subscriptions

    GET current subscription information (for debug purposes)
````

```bash
/v1/scn

    POST a State Change Notification for fanout to subscribers
```

```bash
/v1/params

    GET current _hmnfd_ operating parameters

    PATCH changes to _hmnfd_ operating parameters
```

See https://github.com/Cray-HPE/hms-hmi-nfd/blob/master/api/swagger.yaml for details on the hbtd RESTful API payloads and return values.

## hmnfd Command Line

```bash
Usage: hmnfd [options]

  --debug=num             Debug level (Default: 0)
  --kv_url=num            Key-value service base URL. (Default: mem:)
  --help                  Help text.
  --nosm                  Don't contact State Manager (for debugging).
  --port=num              HTTPS port to listen on. (Default: 28600)
  --sm_retries=num        Number of times to retry on State Manager error. 
                             (Default: 3)
  --sm_timeout=num        Seconds to wait on State Manager accesses. 
                             (Default: 10)
  --sm_url=url            State Manager base URL. 
                             (Default: https://localhost:27999/hsm/v1)
  --telemetry_host=h:p:t  Hostname:port:topic  of telemetry service
  --use_telemetry         Inject notifications onto telemetry bus (Default: no)
```


## Building And Executing hmnfd

### Building hmnfd

[Building _hmnfd_ after the Repo split](https://connect.us.cray.com/confluence/display/CASMHMS/HMS+Repo+Split)

### Running hmnfd Locally

Starting _hmnfd_:

```bash
./hmnfd --sm_url=https://localhost:27999/hsm/smd/v1 --port=28501 --use_telemetry=no --kv_url="mem:"
```

### Running hmnfd In A Docker Container

From the root of this repo, build the docker container:

```bash
# docker build -t cray/cray-hmnfd:test .
```

Then run (add `-d` to the arguments list of `docker run` to run in detached/background mode):

```bash
docker run -p 28500:28500 --name hmnfd cray/hbtd:test
```

## Feature Map

| V1 Feature | V1+ Feature | XC Equivalent |
| --- | --- | --- |
| /v1/subscribe | - | ERD event (e.g. ec_l0_node_dwn) subscription |
| /v1/subscriptions | - | ERD side-door debug to look at subscription info |
| /v1/scn | - | Application-level ERD message send |
| /v1/params | - | - |
| - | Ability to query service health | - |
| - | Ability to dump service internals | - |

## Current Features

* Ability to subscribe for State Change Notifications
* Ability to query subscription information
* Ability to query and modify operating parameters
* Ability to inject state change notification for distribution to subscribers

## Future Features And Updates

* Performance/scaling optimizations
  * SCN fanout algorithm changes to spread the fanout load more evenly
  * Potential K/V query optimizations
* Add API to query service health and connectivity
* Add API to dump service internal

