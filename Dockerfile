# Copyright (c) 2022 - 2026, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

# Build the manager binary
# syntax=docker/dockerfile:1.7
#
# Copyright (c) 2022 - 2026, Oracle and/or its affiliates.
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
ARG GOLANG_VERSION="1.27.4"
ARG DEBUG="false"
ENV GOTOOLCHAIN=local
RUN set -e; \
    current_go_version=""; \
    if command -v go >/dev/null 2>&1; then \
      current_go_version="$(go version | awk '{print $3}')"; \
    fi; \
    if [ "${INSTALL_GO}" = "true" ] || [ "${current_go_version}" != "go${GOLANG_VERSION}" ]; then \
      echo "Installing Go ${GOLANG_VERSION} for linux/${TARGETARCH} (current: ${current_go_version:-missing})"; \
      curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GOLANG_VERSION}.linux-${TARGETARCH}.tar.gz"; \
      rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz; \
      rm -f /tmp/go.tgz; \
    fi; \
    /usr/local/go/bin/go version
ENV PATH=/usr/local/go/bin:${PATH}
ENV GOCACHE=/go-cache
ENV GOMODCACHE=/gomod-cache
ENV GOBIN=/workspace/bin

WORKDIR /workspace

# Ensure Go is on PATH when installed above
ENV PATH="/usr/local/go/bin:${PATH}"

# Copy module manifests first for better caching
COPY  go.mod go.mod
COPY  go.sum go.sum

# Resolve module graph before copying source so dependency drift shows up early
# and source-only edits do not invalidate dependency downloads.
RUN --mount=type=cache,id=oracle-database-operator-gocache,target=/go-cache,sharing=locked \
    --mount=type=cache,id=oracle-database-operator-gomodcache,target=/gomod-cache,sharing=locked \
    set -e; \
    export PATH="/usr/local/go/bin:${PATH}"; \
    export GOTOOLCHAIN=local; \
    /usr/local/go/bin/go version; \
    /usr/local/go/bin/go mod download -x all

# Copy source
COPY  LICENSE.txt LICENSE.txt
COPY  THIRD_PARTY_LICENSES_DOCKER.txt THIRD_PARTY_LICENSES_DOCKER.txt
COPY  main.go main.go
COPY  apis/ apis/
COPY  commons/ commons/
COPY  controllers/ controllers/

RUN --mount=type=cache,id=oracle-database-operator-gocache,target=/go-cache,sharing=locked \
    --mount=type=cache,id=oracle-database-operator-gomodcache,target=/gomod-cache,sharing=locked \
    export PATH="/usr/local/go/bin:${PATH}"; \
    export GOTOOLCHAIN=local; \
    /usr/local/go/bin/go mod download all

# Build manager (debug flags when DEBUG=true) and optionally install dlv
RUN --mount=type=cache,id=oracle-database-operator-gocache,target=/go-cache,sharing=locked \
    --mount=type=cache,id=oracle-database-operator-gomodcache,target=/gomod-cache,sharing=locked \
    set -e; \
    export PATH="/usr/local/go/bin:${PATH}"; \
    export GOTOOLCHAIN=local; \
    /usr/local/go/bin/go version; \
    if [ "${DEBUG}" = "true" ]; then \
      mkdir -p "${GOBIN}"; \
      CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" GO111MODULE=on \
        /usr/local/go/bin/go build -gcflags="all=-N -l" -o /workspace/manager main.go; \
      dlv_build_dir="$(mktemp -d)"; \
      cd "$dlv_build_dir"; \
      /usr/local/go/bin/go mod init dlv-build; \
      /usr/local/go/bin/go get github.com/go-delve/delve/cmd/dlv@v1.27.0; \
      /usr/local/go/bin/go get golang.org/x/sys@v0.47.0; \
      /usr/local/go/bin/go install github.com/go-delve/delve/cmd/dlv; \
      rm -rf "$dlv_build_dir"; \
    else \
      CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" GO111MODULE=on \
        /usr/local/go/bin/go build -o /workspace/manager main.go; \
    fi


# ----------------------------
# Runtime base (shared by prod/debug)
# ----------------------------
FROM ${RUNNER_IMG} AS runtime-base

LABEL provider="Oracle" \
      issues="https://github.com/oracle/oracle-database-operator/issues" \
      maintainer="paramdeep.saini@oracle.com, sanjay.singh@oracle.com, kuassi.mensah@oracle.com" \
      version="2.3" \
      description="DB Operator Image V2.3" \
      vendor="Oracle Corporation" \
      release="2.3" \
      summary="Oracle Database Operator 2.3" \
      name="oracle-database-operator.v2.3"

ARG CI_COMMIT_SHA
ARG CI_COMMIT_BRANCH
ENV COMMIT_SHA="${CI_COMMIT_SHA}" \
    COMMIT_BRANCH="${CI_COMMIT_BRANCH}"

WORKDIR /

# Record RPM provenance for every runtime image.
USER root
RUN microdnf update -y \
    && microdnf clean all \
    && rm -rf /var/cache/dnf /var/cache/yum

RUN mkdir -p /usr/share/oracle/image-metadata/rpms && \
    rpm -qa --qf '%{NAME}-%{VERSION}-%{RELEASE}.%{ARCH}\n' | sort > /usr/share/oracle/image-metadata/rpms/base-image-rpms.txt && \
    rpm -qa --qf '%{NAME}\n' | sort -u > /usr/share/oracle/image-metadata/rpms/base-image-rpm-names.txt

# Create non-root user
RUN useradd -u 1002 nonroot

# Common runtime files
COPY ordssrvs/ords_init.sh /ordssrvs/
COPY ordssrvs/ords_start.sh /ordssrvs/
COPY ordssrvs/RSADecryptOAEP.java /ordssrvs/
COPY ordssrvs/access_log_forwarder.sh /ordssrvs/
COPY  LICENSE.txt /licenses/LICENSE.txt
COPY  THIRD_PARTY_LICENSES_DOCKER.txt /licenses/THIRD_PARTY_LICENSES_DOCKER.txt
COPY  THIRD_PARTY_LICENSES.txt /licenses/THIRD_PARTY_LICENSES.txt

RUN rpm -qa --qf '%{NAME}-%{VERSION}-%{RELEASE}.%{ARCH}\n' | sort > /usr/share/oracle/image-metadata/rpms/image-rpms.txt && \
    rpm -qa --qf '%{NAME}\t%{VERSION}-%{RELEASE}.%{ARCH}\n' | sort > /tmp/image-rpm-details && \
    awk -F '\t' 'NR==FNR {base[$1]=1; next} !($1 in base) {print $1 "-" $2}' \
      /usr/share/oracle/image-metadata/rpms/base-image-rpm-names.txt /tmp/image-rpm-details > \
      /usr/share/oracle/image-metadata/rpms/image-added-rpms.txt && \
    rm -f /tmp/image-rpm-details

USER 1002:1002
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
