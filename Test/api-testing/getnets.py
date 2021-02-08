#!/usr/bin/python
# MIT License
#
# (C) Copyright [2021] Hewlett Packard Enterprise Development LP
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

