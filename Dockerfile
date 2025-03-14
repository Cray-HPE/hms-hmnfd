# MIT License
#
# (C) Copyright [2019-2022,2025] Hewlett Packard Enterprise Development LP
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

# Dockerfile for building Cray-HPE NFD (Node Fanout Daemon).

### build-base stage ###
# Build base just has the packages installed we need.
FROM artifactory.algol60.net/docker.io/library/golang:1.23-alpine AS build-base

RUN set -ex \
    && apk -U upgrade \
    && apk add build-base


### base stage ###
# Base copies in the files we need to test/build.
FROM build-base AS base

RUN go env -w GO111MODULE=auto

# Copy all the necessary files to the image.
COPY cmd $GOPATH/src/github.com/Cray-HPE/hms-hmnfd/cmd
COPY vendor $GOPATH/src/github.com/Cray-HPE/hms-hmnfd/vendor


### Build Stage ###
FROM base AS builder

RUN set -ex && go build -v -tags musl -o /usr/local/bin/hmnfd github.com/Cray-HPE/hms-hmnfd/cmd/hmi-nfd


### Final Stage ###
FROM artifactory.algol60.net/docker.io/alpine:3.21
LABEL maintainer="Hewlett Packard Enterprise"
EXPOSE 28600
STOPSIGNAL SIGTERM

RUN set -ex \
    && apk -U upgrade \
    && apk add --no-cache curl

# Copy the final binary.  

COPY --from=builder /usr/local/bin/hmnfd /usr/local/bin

# Run the daemon.  Note that these env vars are likely to be overridden
# by the Helm chart.

ENV DEBUG=0
ENV SM_URL="https://cray-smd/hsm/v2"
ENV INBOUND_SCN_URL="https://cray-hmnfd/hmi/v1/scn"
ENV SM_RETRIES=3
ENV SM_TIMEOUT=10
ENV PORT=28600
ENV USE_TELEMETRY="--use_telemetry"
ENV TELEMETRY_HOST="cluster-kafka-bootstrap.sma.svc.cluster.local:9092:cray-hmsstatechange-notifications"
ENV NOSM=""

# If KV_URL is set to empty the Go code will determine the URL from env vars.
# This is due to the fact that in Dockerfiles you CANNOT create an env var 
# using other env vars.

ENV KV_URL=

# nobody 65534:65534
USER 65534:65534

CMD ["sh", "-c", "hmnfd --debug=$DEBUG $NOSM --sm_url=$SM_URL --sm_retries=$SM_RETRIES --sm_timeout=$SM_TIMEOUT --port=$PORT --kv_url=$KV_URL --scn_in_url=$INBOUND_SCN_URL $USE_TELEMETRY --telemetry_host=$TELEMETRY_HOST"]
