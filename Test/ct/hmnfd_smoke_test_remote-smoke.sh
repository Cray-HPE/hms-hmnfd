#!/bin/bash -l
#
# Copyright 2020 Hewlett Packard Enterprise Development LP
#
###############################################################
#
#     CASM Test - Cray Inc.
#
#     TEST IDENTIFIER   : hmnfd_smoke_test
#
#     DESCRIPTION       : Automated test for verifying basic HMNFD API
#                         infrastructure and installation on Cray Shasta
#                         systems.
#                         
#     AUTHOR            : Mitch Schooler
#
#     DATE STARTED      : 09/23/2020
#
#     LAST MODIFIED     : 09/23/2020
#
#     SYNOPSIS
#       This is a smoke test for the HMS HMNFD API that makes basic HTTP
#       requests using curl to verify that the service's API endpoints
#       respond and function as expected after an installation.
#
#     INPUT SPECIFICATIONS
#       Usage: hmnfd_smoke_test
#       
#       Arguments: None
#
#     OUTPUT SPECIFICATIONS
#       Plaintext is printed to stdout and/or stderr. The script exits
#       with a status of '0' on success and '1' on failure.
#
#     DESIGN DESCRIPTION
#       This smoke test is based on the Shasta health check srv_check.sh
#       script in the CrayTest repository that verifies the basic health of
#       various microservices but instead focuses exclusively on the HMNFD 
#       API. It was implemented to run from the ct-pipelines container off
#       of the NCN of the system under test within the DST group's Continuous
#       Testing (CT) framework as part of the remote-smoke test suite.
#
#     SPECIAL REQUIREMENTS
#       Must be executed from the ct-pipelines container on a remote host
#       (off of the NCNs of the test system) with the Continuous Test
#       infrastructure installed.
#
#     UPDATE HISTORY
#       user       date         description
#       -------------------------------------------------------
#       schooler   09/23/2020   initial implementation
#
#     DEPENDENCIES
#       - hms_smoke_test_lib_ncn-resources_remote-resources.sh which is
#         expected to be packaged in
#         /opt/cray/tests/remote-resources/hms/hms-test in the ct-pipelines
#         container.
#
#     BUGS/LIMITATIONS
#       None
#
###############################################################

# HMS test metrics test cases: 6
# 1. Check cray-hmnfd pod statuses
# 2. GET /health API response code
# 3. GET /liveness API response code
# 4. GET /readiness API response code
# 5. GET /subscriptions API response code
# 6. GET /params API response code

# initialize test variables
TEST_RUN_TIMESTAMP=$(date +"%Y%m%dT%H%M%S")
TEST_RUN_SEED=${RANDOM}
OUTPUT_FILES_PATH="/tmp/hmnfd_smoke_test_out-${TEST_RUN_TIMESTAMP}.${TEST_RUN_SEED}"
SMOKE_TEST_LIB="/opt/cray/tests/remote-resources/hms/hms-test/hms_smoke_test_lib_ncn-resources_remote-resources.sh"
CURL_ARGS="-k -i -s -S"
MAIN_ERRORS=0
CURL_COUNT=0

# cleanup
function cleanup()
{
    echo "cleaning up..."
    rm -f ${OUTPUT_FILES_PATH}*
}

# main
function main()
{
    AUTH_ARG="-H \"Authorization: Bearer $TOKEN\""

    # GET tests
    for URL_ARGS in \
        "apis/hmnfd/hmi/v1/health" \
        "apis/hmnfd/hmi/v1/liveness" \
        "apis/hmnfd/hmi/v1/readiness" \
        "apis/hmnfd/hmi/v1/subscriptions" \
        "apis/hmnfd/hmi/v1/params"
    do
        URL=$(url "${URL_ARGS}")
        URL_RET=$?
        if [[ ${URL_RET} -ne 0 ]] ; then
            cleanup
            exit 1
        fi
        run_curl "GET ${AUTH_ARG} ${URL}"
        if [[ $? -ne 0 ]] ; then
            ((MAIN_ERRORS++))
        fi
    done

    echo "MAIN_ERRORS=${MAIN_ERRORS}"
    return ${MAIN_ERRORS}
}

# check_pod_status
function check_pod_status()
{
    run_check_pod_status "cray-hmnfd"
    return $?
}

# TARGET_SYSTEM is expected to be set in the ct-pipelines container
if [[ -z ${TARGET_SYSTEM} ]] ; then
    >&2 echo "ERROR: TARGET_SYSTEM environment variable is not set"
    cleanup
    exit 1
else
    echo "TARGET_SYSTEM=${TARGET_SYSTEM}"
    TARGET="auth.${TARGET_SYSTEM}"
    echo "TARGET=${TARGET}"
fi

# TOKEN is expected to be set in the ct-pipelines container
if [[ -z ${TOKEN} ]] ; then
    >&2 echo "ERROR: TOKEN environment variable is not set"
    cleanup
    exit 1
else
    echo "TOKEN=${TOKEN}"
fi

trap ">&2 echo \"recieved kill signal, exiting with status of '1'...\" ; \
    cleanup ; \
    exit 1" SIGHUP SIGINT SIGTERM

# source HMS smoke test library file
if [[ -r ${SMOKE_TEST_LIB} ]] ; then
    . ${SMOKE_TEST_LIB}
else
    >&2 echo "ERROR: failed to source HMS smoke test library: ${SMOKE_TEST_LIB}"
    exit 1
fi

# make sure filesystem is writable for output files
touch ${OUTPUT_FILES_PATH}
if [[ $? -ne 0 ]] ; then
    >&2 echo "ERROR: output file location not writable: ${OUTPUT_FILES_PATH}"
    cleanup
    exit 1
fi

echo "Running hmnfd_smoke_test..."

# run initial pod status test
check_pod_status
if [[ $? -ne 0 ]] ; then
    echo "FAIL: hmnfd_smoke_test ran with failures"
    cleanup
    exit 1
fi

# run main API tests
main
if [[ $? -ne 0 ]] ; then
    echo "FAIL: hmnfd_smoke_test ran with failures"
    cleanup
    exit 1
else
    echo "PASS: hmnfd_smoke_test passed!"
    cleanup
    exit 0
fi
