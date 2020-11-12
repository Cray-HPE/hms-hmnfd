#!/usr/bin/python

# Simple utility to examine the docker network details and extract
# the container names and IP addresses.  This is used for adding
# /etc/hosts entries on the Tavern test container, allowing it to
# talk to all of the containers in our docker-compose - built 
# hmnfd test container set.
#
# This util takes data from stdin, which comes from the command:
#
#   docker docker network inspect hms-services_ttest
#
# hms-services_ttest is the network used by the docker-compose
# container set.  The Tavern test container is not part of this
# container set, so it has to be manually hooked up to that network
# and told the hostnames of the container set services.
#
# The output of this util is a set of docker command line arguments
# in the form:
#
#  --add-host=ip_addr:hostname --add-host=ip_addr:hostname ...


import sys,json;

stuff = json.load(sys.stdin)
buf = ""

for cont in stuff[0]['Containers']:
    name = stuff[0]['Containers'][cont]['Name']
    addy = stuff[0]['Containers'][cont]['IPv4Address']
    addr = addy[:-3]
    buf += "--add-host=%s:%s " % (name,addr)

print buf

