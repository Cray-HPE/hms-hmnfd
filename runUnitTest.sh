#!/usr/bin/env bash

# MIT License
#
# (C) Copyright [2020-2021] Hewlett Packard Enterprise Development LP
#
# Permission is hereby granted, free of charge, to any person obtaining a
# copy of this software and associated documentation files (the "Software"),
# to deal in the Software without restriction, including without limitation
# the rights to use, copy, modify, merge, publish, distribute, sublicense,
# and/or sell copies of the Software, and to permit persons to whom the
# Software is furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included
# in all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
# THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
# OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
# OTHER DEALINGS IN THE SOFTWARE.

# This script is run from Jenkins when code is commited for hmnfd.
# It can also be run standalone.
#
# The purpose of this script is to run a set of API and functional tests.
# The components used in this test setup are:
#
# o A container running hmnfd
# o A container running a fake State Manager
# o Two containers running fake nodes/SCN subscribers
# o A container running ETCD
#
# The above containers are built into a container set using 'docker-compose'.
#
# Then, another container is used which contains Tavern and a set of
# tests that Tavern runs.  This container is built, and uses the RUN directive
# to execute the tests, giving us pass/fail return value.
#
# The sequence of events for this testing is:
#
# 1. hmnfd, fake State Manager, and fake SCN subscriber containers are built 
#    from current source.  We don't pull them from Artifactory, since we want
#    to test everything that was just checked in; using Artifactory gets the
#    previous versions.
#
# 2. A container set is built using docker-compose.  It will build the set
#    containing the hmnfd, fake State Manager, fake SCN subscriber containers
#    and an ETCD container pulled from bitnami.
#
# 3. Container set network information and host names are fetched from 
#    docker and fed into a script which spits out the appropriate --add-hosts
#    arguments for 'docker build'
#
# 4. The Tavern test container is built, which runs the Tavern tests and gives
#    us a pass/fail result, which is returned to Jenkins.

DOCKERFILEZ="Dockerfile.fake-hsm Dockerfile.fake-subscriber Dockerfile.hmnfd-apitest docker-compose-hmnfdapitest.yaml"
BASEPORT=25000
testtag=apitest
tagsuffix=${RANDOM}_${RANDOM}
export HTAG=${testtag}
export HSUFFIX=${tagsuffix}

cleanup_containers() {
    # Get rid of the interim containers

    echo " "
    echo "=============== > Deleting temporary containers..."
    echo " "

    for fff in `docker images | grep ${HTAG}_${HSUFFIX} | awk '{printf("%s\n",$3)}'`; do
        docker image rm -f ${fff}
    done

    # Remove temporary symlinks
    for fff in `echo ${DOCKERFILEZ}`; do
        rm -f ${fff}
    done
}

# Get list of ports to use.  We'll index our ports off of the number of
# currently running runUnitTest.sh processes to avoid collisions.

nrut=`pgrep -c -f runUnitTest.sh`
(( PORTBASE = BASEPORT + nrut ))

echo "PORT INDEX: ${nrut}"

(( FAKE_SM_PORT = PORTBASE+0 ))
(( FAKE_SUBA_PORT = PORTBASE+1000 ))
(( FAKE_SUBB_PORT = PORTBASE+2000 ))
(( FAKE_ETCD_A_PORT = PORTBASE+3000 ))
(( FAKE_HMNFD_PORT = PORTBASE+4000 ))
export FAKE_SM_PORT
export FAKE_SUBA_PORT
export FAKE_SUBB_PORT
export FAKE_ETCD_A_PORT
export FAKE_HMNFD_PORT

# It's possible we don't have docker-compose, so if necessary bring our own.

docker_compose_file=./docker-compose-hmnfdapitest.yaml
docker_compose_exe=$(command -v docker-compose)

if ! [[ -x "$docker_compose_exe" ]]; then
    if ! [[ -x "./docker-compose" ]]; then
        echo "Getting docker-compose..."
        curl -L "https://github.com/docker/compose/releases/download/1.23.2/docker-compose-$(uname -s)-$(uname -m)" \
        -o ./docker-compose

        if [[ $? -ne 0 ]]; then
            echo "Failed to fetch docker-compose!"
            exit 1
        fi

        chmod +x docker-compose
    fi
    docker_compose_exe="./docker-compose"
fi

# Build and run a container set containing the following containers:
#   o hmnfd with specific cmdline args
#   o A faked-out HSM
#   o 2 fake node/SCN subscribers
#   o An ETCD cluster container
#
# Note that we'll use our own project name, since Jenkins sometimes does
# wierd stuff like putting the project into a directory that starts with a
# dash, which wreaks havoc on Docker tools

echo " "
echo "=============== > BUILD docker-compose container set..."
echo " "

proj_name=hmnfd_${tagsuffix}
logfilename=testcluster_${proj_name}.logs

# Set up temporary soft links to Dockerfiles in the test area.  
# This helps docker-compose find all the stuff it needs.  These will
# get cleaned up in the normal cleanup function.  The links are 
# needed for both the build and run stages.

for fff in `echo ${DOCKERFILEZ}`; do
    rm -f ${fff}
    ln -s Test/api-testing/${fff}
done

DCOMPOSE="${docker_compose_exe} -p ${proj_name} -f ${docker_compose_file}"

${DCOMPOSE} build
drval=$?

if [[ $drval -ne 0 ]]; then
    cleanup_containers
    echo "Docker compose build FAILED, exiting."
    exit 1
fi

echo " "
echo "=============== > RUN docker-compose container set..."
echo " "
${DCOMPOSE} up -d

if [[ $? -ne 0 ]]; then
    # Not sure this will do anything at this point, but just in case.
    ${DCOMPOSE} logs > ${logfilename} 2>&1
    ${DCOMPOSE} down
    cleanup_containers
    echo "Docker compose up FAILED, exiting."
    exit 1
fi

# Now find the network we want to use.  It will have a suffix of "_ttest".
# Note that running standalone versus in Jenkins will have different
# names, but in any case the one that ends in _ttest is the one we want.

container_network=`docker network ls --filter "name=${HSUFFIX}_ttest" --format "{{.Name}}"`
echo "Bridge network name: ${container_network}"
docker network inspect ${container_network}

# Everything is ready.  Now just build the TAVERN test container, which
# will run the tests.  We'll specify the hosts/ips of the bridge network
# services running in the cluster in the building/running our TAVERN test 
# container.

addhosts=`docker network inspect ${container_network} | ./Test/api-testing/getnets.py`

if [[ "${addhosts}" == "" ]]; then
    echo "No containers/network data found in docker network for our services, exiting."
    ${DCOMPOSE} logs > ${logfilename} 2>&1
    ${DCOMPOSE} down
    cleanup_containers
    exit 1
fi

# Create common.yaml file.  This is because common.yaml can't do variable
# substitution (even though all other .yaml files can...)

cat << COMMON > Test/api-testing/common.yaml
# Note that the host names are set up for running in Docker containers
# where most of the containers are running within a docker-compose framework.
# To run this outside of the containers, the hostnames in the URLs below
# will have to change, most likely to 'localhost'.

name: Fanout Daemon Test Information
description: This file contains common definitions used by TAVERN .yaml files

variables:
  hmnfd_url: http://hmnfd_${HSUFFIX}:${FAKE_HMNFD_PORT}/hmi
  kv_url: http://etcd_server_${HSUFFIX}:${FAKE_ETCD_A_PORT}/v3alpha/kv/deleterange
  n0_sub_url: http://fake_subscriber_a_${HSUFFIX}:${FAKE_SUBA_PORT}/x0c0s0b0n0
  n1_sub_url: http://fake_subscriber_b_${HSUFFIX}:${FAKE_SUBB_PORT}/x0c0s0b0n1
COMMON

echo " "
echo "=============== > Building/running: TAVERN test container..."
echo " "

echo "Addhosts: .${addhosts}."
echo "NW: .${container_network}."
echo "Running: DOCKER_BUILDKIT=0 docker build --rm --no-cache --network=${container_network} ${addhosts} -f Test/api-testing/Dockerfile.tavern ."

DOCKER_BUILDKIT=0 docker build --rm --no-cache --network=${container_network} ${addhosts} -f Test/api-testing/Dockerfile.tavern .

test_rslt=$?

# Shut down and clean up

rm -f Test/api-testing/common.yaml

echo " "
echo "=============== > Shutting down container set..."
echo " "

${DCOMPOSE} logs > ${logfilename} 2>&1
${DCOMPOSE} down
cleanup_containers

echo " "
echo " See 'testcluster.logs' for container set logs."
echo " "

echo " "
echo "================================================="
if [[ ${test_rslt} -ne 0 ]]; then
    echo "TAVERN test(s) FAILED."
else
    echo "TAVERN tests SUCCESS!"
fi
echo "================================================="

exit ${test_rslt}
