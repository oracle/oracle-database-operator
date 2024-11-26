# Copyright (c) 2022, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

# Build the manager binary
ARG BUILDER_IMG
FROM ${BUILDER_IMG} as builder

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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o manager main.go

# Use oraclelinux:9 as base image to package the manager binary
FROM oraclelinux:9
ARG CI_COMMIT_SHA 
ARG CI_COMMIT_BRANCH
ENV COMMIT_SHA=${CI_COMMIT_SHA} \
    COMMIT_BRANCH=${CI_COMMIT_BRANCH}
WORKDIR /
COPY --from=builder /workspace/manager .
RUN useradd -u 1002 nonroot
USER nonroot

ENTRYPOINT ["/manager"]
