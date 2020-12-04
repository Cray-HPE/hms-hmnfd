#!/bin/sh

# This script is included in the tavern test container built by
# Dockerfile.hmi-nfd-taverntest.  It runs all of the TAVERN-based
# tests for hmnfd API and functional testing using a docker-compose
# container set containing:

YAMLDIR=/usr/local/bin

echo "Copying YAML files..."
cp ${YAMLDIR}/*.yaml .


# run /params test

pytest ./test_hmnfd_api_params.tavern.yaml 
if [ $? -ne 0 ]; then
    echo " "
    echo ">>>>> ERROR: /params API test failed! <<<<<<<<<"
    exit 1
fi

# run /subscribe and /subscriptions test

pytest ./test_hmnfd_api_subscribe.tavern.yaml 
if [ $? -ne 0 ]; then
    echo " "
    echo ">>>>> ERROR: /subscribe, /subscriptions API test failed! <<<<<<<<<"
    exit 1
fi

# run /scn test

pytest ./test_hmnfd_api_scn.tavern.yaml 
if [ $? -ne 0 ]; then
    echo " "
    echo ">>>>> ERROR: /scn API test failed! <<<<<<<<<"
    exit 1
fi

