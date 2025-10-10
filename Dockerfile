# Copyright (c) 2022, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

# Build the manager binary
ARG BUILDER_IMG="oraclelinux:9"
ARG RUNNER_IMG="oraclelinux:9-slim"
FROM ${BUILDER_IMG} AS builder

ARG TARGETARCH
# Download golang if INSTALL_GO is set to true
ARG INSTALL_GO
ARG GOLANG_VERSION
RUN if [ "$INSTALL_GO" = "true" ]; then \
        echo -e "\nCurrent Arch: $(arch), Downloading Go for linux/${TARGETARCH}" &&\
        curl -LJO https://go.dev/dl/go${GOLANG_VERSION}.linux-${TARGETARCH}.tar.gz &&\
        rm -rf /usr/local/go && tar -C /usr/local -xzf go${GOLANG_VERSION}.linux-${TARGETARCH}.tar.gz &&\
        rm go${GOLANG_VERSION}.linux-${TARGETARCH}.tar.gz; \
        echo "Go Arch: $(/usr/local/go/bin/go env GOARCH)"; \
    fi
ENV PATH=${GOLANG_VERSION:+"${PATH}:/usr/local/go/bin"}
ENV GOCACHE=/go-cache
ENV GOMODCACHE=/gomod-cache

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY LICENSE.txt LICENSE.txt
COPY THIRD_PARTY_LICENSES_DOCKER.txt THIRD_PARTY_LICENSES_DOCKER.txt
COPY main.go main.go
COPY apis/ apis/
COPY commons/ commons/
COPY controllers/ controllers/

# Build
RUN --mount=type=cache,target=/go-cache --mount=type=cache,target=/gomod-cache CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GO111MODULE=on go build -o manager main.go

# Use oraclelinux:9-slim as default base image to package the manager binary
FROM ${RUNNER_IMG}
# Labels
# ------
LABEL "provider"="Oracle"                                                                                                        \
      "issues"="https://github.com/oracle/oracle-database-operator/issues"                                                       \
      "maintainer"="paramdeep.saini@oracle.com, sanjay.singh@oracle.com, kuassi.mensah@oracle.com"                               \
      "version"="2.0"                                                                                                            \
      "description"="DB Operator Image V2.0"                                                                                     \
      "vendor"="Oracle Coporation"                                                                                               \
      "release"="2.0"                                                                                                            \
      "summary"="Oracle Database Operator 2.0"                                                                                  \
      "name"="oracle-database-operator.v2.0"
ARG CI_COMMIT_SHA 
ARG CI_COMMIT_BRANCH
ENV COMMIT_SHA=${CI_COMMIT_SHA} \
    COMMIT_BRANCH=${CI_COMMIT_BRANCH}
WORKDIR /
COPY --from=builder /workspace/manager .
COPY ords/ords_init.sh .
COPY ords/ords_start.sh .
COPY LICENSE.txt /licenses/
COPY THIRD_PARTY_LICENSES_DOCKER.txt /licenses/
COPY THIRD_PARTY_LICENSES.txt /licenses/
RUN useradd -u 1002 nonroot
USER nonroot

ENTRYPOINT ["/manager"]
