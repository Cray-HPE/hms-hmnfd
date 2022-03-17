# MIT License
#
# (C) Copyright [2019-2022] Hewlett Packard Enterprise Development LP
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

# Dockerfile for building HMS fake node/SCN subscriber for testing.
# Author: mpkelly
# Date: 12-February 2019

FROM artifactory.algol60.net/docker.io/library/golang:1.16-alpine AS builder

RUN go env -w GO111MODULE=auto

COPY test/fake-subscriber/fake-subscriber.go ${GOPATH}/src/fake-subscriber/

RUN set -ex && go build -v -tags musl -i -o /usr/local/bin/fake-subscriber fake-subscriber

### Final Stage ###

FROM artifactory.algol60.net/docker.io/alpine:3.15
LABEL maintainer="Hewlett Packard Enterprise"
STOPSIGNAL SIGTERM

# Copy the final binary.  

COPY --from=builder /usr/local/bin/fake-subscriber /usr/local/bin

# Run the fake-subscriber daemon.  Env vars come from the .yaml file.

CMD ["sh", "-c", "fake-subscriber"]