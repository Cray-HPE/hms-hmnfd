# Copyright 2019-2020 Hewlett Packard Enterprise Development LP

# Dockerfile for building HMS NFD (Node Fanout Daemon).

# Build base just has the packages installed we need.
FROM dtr.dev.cray.com/baseos/golang:1.14-alpine3.12 AS build-base

RUN set -ex \
    && apk update \
    && apk add build-base

# Base copies in the files we need to test/build.
FROM build-base AS base

# Copy all the necessary files to the image.
COPY cmd $GOPATH/src/stash.us.cray.com/HMS/hms-hmi-nfd/cmd
COPY vendor $GOPATH/src/stash.us.cray.com/HMS/hms-hmi-nfd/vendor


### Unit Test Stage ###
FROM base AS testing

# Run unit tests...
CMD ["sh", "-c", "set -ex && go test -v stash.us.cray.com/HMS/hms-hmi-nfd/cmd/hmi-nfd"]


### Coverage Stage ###
FROM base AS coverage

# Run test coverage...
CMD ["sh", "-c", "set -ex && go test -cover -v stash.us.cray.com/HMS/hms-hmi-nfd/cmd/hmi-nfd"]


### Build Stage ###
FROM base AS builder

RUN set -ex && go build -v -i -o /usr/local/bin/hmnfd stash.us.cray.com/HMS/hms-hmi-nfd/cmd/hmi-nfd


### Final Stage ###
FROM dtr.dev.cray.com/baseos/alpine:3.12
LABEL maintainer="Cray, Inc."
EXPOSE 28600
STOPSIGNAL SIGTERM

RUN set -ex \
    && apk update \
    && apk add --no-cache curl

# Copy the final binary.  

COPY --from=builder /usr/local/bin/hmnfd /usr/local/bin

# Run the daemon.  Note that these env vars are likely to be overridden
# by the Helm chart.

ENV DEBUG=0
ENV SM_URL="https://cray-smd/hsm/v1"
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

CMD ["sh", "-c", "hmnfd --debug=$DEBUG $NOSM --sm_url=$SM_URL --sm_retries=$SM_RETRIES --sm_timeout=$SM_TIMEOUT --port=$PORT --kv_url=$KV_URL --scn_in_url=$INBOUND_SCN_URL $USE_TELEMETRY --telemetry_host=$TELEMETRY_HOST"]
