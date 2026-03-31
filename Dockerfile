# Copyright (c) 2022, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

# Build the manager binary
# syntax=docker/dockerfile:1.7
#
# Copyright (c) 2022, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

ARG BUILDER_IMG="oraclelinux:9"
ARG RUNNER_IMG="oraclelinux:9-slim"

# ----------------------------
# Builder stage
# ----------------------------
FROM ${BUILDER_IMG} AS builder

ARG TARGETARCH
ARG INSTALL_GO="false"
ARG GOLANG_VERSION
ARG DEBUG="false"

# Go build/cache locations (keeps layers smaller and build faster with BuildKit cache mounts)
ENV GOCACHE=/go-cache \
    GOMODCACHE=/gomod-cache \
    GOBIN=/workspace/bin

WORKDIR /workspace

# Install Go only when requested (for oraclelinux base); if BUILDER_IMG is golang:*, Go is already present.
RUN if [ "${INSTALL_GO}" = "true" ]; then \
      echo "Installing Go ${GOLANG_VERSION} for linux/${TARGETARCH}"; \
      curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GOLANG_VERSION}.linux-${TARGETARCH}.tar.gz"; \
      rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz; \
      rm -f /tmp/go.tgz; \
    fi

# Ensure Go is on PATH when installed above
ENV PATH="/usr/local/go/bin:${PATH}"

# Copy module manifests first for better caching
COPY  go.mod go.mod
COPY  go.sum go.sum

# Resolve module graph before copying source so dependency drift shows up early
# and source-only edits do not invalidate dependency downloads.
RUN --mount=type=cache,target=/go-cache \
    --mount=type=cache,target=/gomod-cache \
    set -e; \
    go mod download

# Copy source
COPY  LICENSE.txt LICENSE.txt
COPY  THIRD_PARTY_LICENSES_DOCKER.txt THIRD_PARTY_LICENSES_DOCKER.txt
COPY  main.go main.go
COPY  apis/ apis/
COPY  commons/ commons/
COPY  controllers/ controllers/

# Build manager (debug flags when DEBUG=true) and optionally install dlv
RUN --mount=type=cache,target=/go-cache \
    --mount=type=cache,target=/gomod-cache \
    set -e; \
    if [ "${DEBUG}" = "true" ]; then \
      CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" GO111MODULE=on \
        go build -gcflags="all=-N -l" -o /workspace/manager main.go; \
      go install github.com/go-delve/delve/cmd/dlv@v1.26.1; \
    else \
      CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" GO111MODULE=on \
        go build -o /workspace/manager main.go; \
    fi


# ----------------------------
# Runtime base (shared by prod/debug)
# ----------------------------
FROM ${RUNNER_IMG} AS runtime-base

LABEL provider="Oracle" \
      issues="https://github.com/oracle/oracle-database-operator/issues" \
      maintainer="paramdeep.saini@oracle.com, sanjay.singh@oracle.com, kuassi.mensah@oracle.com" \
      version="2.0" \
      description="DB Operator Image V2.0" \
      vendor="Oracle Corporation" \
      release="2.0" \
      summary="Oracle Database Operator 2.0" \
      name="oracle-database-operator.v2.0"

ARG CI_COMMIT_SHA
ARG CI_COMMIT_BRANCH
ENV COMMIT_SHA="${CI_COMMIT_SHA}" \
    COMMIT_BRANCH="${CI_COMMIT_BRANCH}"

WORKDIR /

# Create non-root user
RUN useradd -u 1002 nonroot
USER nonroot

# Common runtime files
COPY  ordssrvs/ords_init.sh /ords_init.sh
COPY  ordssrvs/ords_start.sh /ords_start.sh
COPY  LICENSE.txt /licenses/LICENSE.txt
COPY  THIRD_PARTY_LICENSES_DOCKER.txt /licenses/THIRD_PARTY_LICENSES_DOCKER.txt
COPY  THIRD_PARTY_LICENSES.txt /licenses/THIRD_PARTY_LICENSES.txt

ENTRYPOINT ["/manager"]


# ----------------------------
# Debug image (includes dlv)
# ----------------------------
FROM runtime-base AS debug

# manager binary
COPY --from=builder /workspace/manager /manager
# dlv is installed only when DEBUG=true in builder; therefore build debug target with --build-arg DEBUG=true
COPY --from=builder /workspace/bin/dlv /dlv


# ----------------------------
# Prod image (no dlv)
# ----------------------------
FROM runtime-base AS prod

COPY --from=builder /workspace/manager /manager
