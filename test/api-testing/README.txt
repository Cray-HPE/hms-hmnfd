This directory contains files to support automated API and functional
tests for hmnfd.

This test suite uses helper apps that mimic production software; thus
it is not meant to run on a "live" system.  Doing this exercises much
more of the hmnfd logic than running on a live system.

The pieces involved in this testing are:

 o hmnfd
 o A fake State Manager, called fake-hsm
 o A fake node/SCN subscriber, called fake-subscriber
 o ETCD
 o TAVERN and supporting yaml files to direct the testing.

The test procedure is as follows:

 1. Docker containers are built from the most recently checked-in
    source:

    o hmnfd, using a special Dockerfile, which tweaks the hmnfd
      operating parameters for optimal testability

    o fake-hsm, which receives SCN subscriptions from hmnfd

    o fake-subscriber, which acts like a CLE node.  This app subscribes
      to SCNs via hmnfd, provides a REST endpoint to deliver the SCNs
      to, and also has APIs to query SCNs sent (which real nodes won't
      have), again, to enhance testability and increase hmnfd code coverage.

2. A container cluster is constructed using docker-compose.  This spins up
   the hmnfd, fake-hsm, 2 copies of fake-subscriber, and ETCD (straight from
   bitnami).  It also specifies a bridge network called "hms-services_ttest".
   This cluster is spun up in the background and left running until testing
   is completed.

3. The cluster network is queried via a docker command to get hostnames
   and IP addresses to be used by the remaining container so that it can
   access the services in the cluster by name.

4. The TAVERN test container is built.  The Dockerfile for this container
   uses a RUN command to run the TAVERN tests contained in it, which returns
   a pass/fail status to the test framework.

5. The cluster is taken down.

FILES

The following files are used by this test process.  All files are located in
hms-services/go/src/hss/hmi-nfd/Test except where noted.

  hmnfd_runUnitTest.sh         # Overall script used by Jenkins (can be run
                               # standalone too).  This file must be specified
                               # by the 'unitTestScript' directive in the
                               # Jenkinsfile).  Located in hms-services
                               # directory

  getnets.py                   # Helper app to create host/IP mappings from
                               # a running test cluster.

  runTavernTests.sh            # Script put into the Tavern test container to 
                               # run all TAVERN tests

  Dockerfile-hmnfd-apitest     # Builds the hmnfd test container
  Dockerfile.fake-subscriber   # Builds the fake-subscriber test container
  Dockerfile.fake-hsm          # Builds the fake HSM test container
  Dockerfile.tavern            # Builds the TAVERN test container

  common.yaml                          # Parameters used by all tests
  test_hmnfd_api_params.tavern.yaml    # Run hmnfd /params API tests
  test_hmnfd_api_scn.tavern.yaml       # Run hmnfd /scn API tests
  test_hmnfd_api_subscribe.tavern.yaml # Runs hmnfd /subscribe and 
                                         /subscriptions API tests

  testcluster.logs                     # stdout/stderr of test cluster services
                                       # Ends up in hms-services directory.


The hmnfd_runUnitTest.sh script is located in the hms-services directory and
will copy needed files into the hms-services directory and run everything, 
cleaning up after itself.  The TAVERN test puts its data onto stdout.  The 
test cluster services (hmnfd, fake-xxx) will dump their output into a file 
called 'testcluster.logs'.

FUTURE WORK

These tests exercise quite a lot of hmnfd.  More tests can be added with
more error injection/testing in the APIs.  These files can be used as a basis
for writing a suite of "on-line" hmnfd tests which are non-destructive.

However, note that any non-destructive test will not test a huge amount of
the hmnfd logic.   The most important part of hmnfd is the fanout of SCNs
to subscribers.  Real-world nodes/SCN subscribers will not provide any way
to query results or details of delivery.  The tests will be useful, but
not really cover anything not already covered by boot testing.


