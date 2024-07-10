# Copyright (c) 2022, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

# Build the manager binary
ARG BUILDER_IMG
FROM ${BUILDER_IMG} as builder

# Download golang if BUILD_INTERNAL is set to true
ARG INSTALL_GO
ARG GOLANG_VERSION
RUN if [ "$INSTALL_GO" = "true" ]; then \
        curl -LJO https://go.dev/dl/go${GOLANG_VERSION}.linux-amd64.tar.gz &&\
        rm -rf /usr/local/go && tar -C /usr/local -xzf go${GOLANG_VERSION}.linux-amd64.tar.gz &&\
        rm go${GOLANG_VERSION}.linux-amd64.tar.gz; \
    fi
ENV PATH=${GOLANG_VERSION:+"${PATH}:/usr/local/go/bin"}

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY apis/ apis/
COPY controllers/ controllers/
COPY commons/ commons/
COPY LICENSE.txt LICENSE.txt
COPY THIRD_PARTY_LICENSES_DOCKER.txt THIRD_PARTY_LICENSES_DOCKER.txt

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go

# Use oraclelinux:8-slim as base image to package the manager binary
FROM oraclelinux:8
ARG CI_COMMIT_SHA 
ARG CI_COMMIT_BRANCH
ENV COMMIT_SHA=${CI_COMMIT_SHA} \
    COMMIT_BRANCH=${CI_COMMIT_BRANCH}
WORKDIR /
COPY --from=builder /workspace/manager .

ARG TT_UID=3429
ARG TT_USER=timesten
ARG TT_GID=3429
ARG TT_GROUP=timesten
ARG TT_RELEASE=22.1.1.22.0

RUN dnf -y install openssl unzip tar gzip perl libaio ncurses-compat-libs libnsl nmap-ncat perl-JSON-PP bind-utils

#TODO: add nonroot user and 1002 group, maybe it is usable by the rdbms 

RUN groupadd -g $TT_GID $TT_GROUP && useradd -m -s /bin/bash -u $TT_UID -g $TT_GROUP $TT_USER 
RUN install -d -m 0750 -o $TT_USER -g $TT_GROUP /timesten /timesten/installations /timesten/operators /timesten/operators/tt$TT_RELEASE && install -d -m 0755 -o $TT_USER -g $TT_GROUP /timesten/installations/tt$TT_RELEASE 
RUN install -d -m 0750 -o $TT_USER -g $TT_GROUP /home/timesten /timesten /timesten/installations /timesten/operators /timesten/operators/tt22.1.1.22.0 
RUN install -d -m 0755 -o $TT_USER -g $TT_GROUP /home/timesten /timesten/installations/tt22.1.1.22.0 
RUN ln -s /timesten/installations/tt22.1.1.22.0 /timesten/installation  
RUN ln -s /timesten/operators/tt22.1.1.22.0 /timesten/
EXPOSE 6624 6625 3754

#TODO what is setuproot for ? 
#RUN /timesten/instance1/bin/setuproot -install


USER $TT_UID:$TT_GID

ENTRYPOINT ["/manager"]
