ARG UBUNTU_RELEASE=resolute
ARG NODE_VERSION=24
ARG GO_VERSION=1.25.13
ARG ONNX_RUNTIME_VERSION=1.22.0
ARG ONNX_RUNTIME_SHA256=8344d55f93d5bc5021ce342db50f62079daf39aaafb5d311a451846228be49b3

# Keep the frontend and backend toolchains identical between development and
# production builds. The runtime libraries themselves are defined once below.
FROM node:${NODE_VERSION}-bookworm-slim AS node-toolchain
FROM golang:${GO_VERSION}-bookworm AS go-toolchain

#
# svelte-builder
#

FROM node-toolchain AS app-builder

WORKDIR /app
ARG VITE_XIVI_SOURCE_URL
ENV VITE_XIVI_SOURCE_URL=${VITE_XIVI_SOURCE_URL}
COPY .npmrc package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm,sharing=locked \
    npm ci --ignore-scripts

# Keep backend-only changes from invalidating the frontend build.
COPY svelte.config.js tsconfig.json vite.config.ts ./
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md ./
COPY scripts/legal.mjs /app/scripts/legal.mjs
COPY src/ /app/src
COPY static/ /app/static
RUN npx svelte-kit sync && npm run build

#
# shared native runtime
#

FROM ubuntu:${UBUNTU_RELEASE} AS native-runtime

ARG ONNX_RUNTIME_VERSION
ARG ONNX_RUNTIME_SHA256
ARG DEBIAN_FRONTEND=noninteractive

# This is the single source of truth for native packages used by both the
# production image and the development container. In particular, GStreamer
# and all of its plugin families always come from the same Ubuntu release.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-ugly \
    gstreamer1.0-tools \
    libglib2.0-0 \
    libgstreamer1.0-0 \
    libvips42t64 \
    openssl \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Install ONNX Runtime once so development and production use the same files.
RUN curl -fsSL \
    "https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_RUNTIME_VERSION}/onnxruntime-linux-x64-${ONNX_RUNTIME_VERSION}.tgz" \
    -o /tmp/onnxruntime.tgz \
    && echo "${ONNX_RUNTIME_SHA256}  /tmp/onnxruntime.tgz" | sha256sum -c - \
    && mkdir -p /usr/local/onnxruntime \
    && tar -xzf /tmp/onnxruntime.tgz -C /usr/local/onnxruntime --strip-components=1 \
    && rm /tmp/onnxruntime.tgz \
    && ldconfig /usr/local/onnxruntime/lib

ENV LD_LIBRARY_PATH="/usr/local/onnxruntime/lib" \
    ONNX_PATH="/usr/local/onnxruntime/lib/libonnxruntime.so"

#
# native build environment
#

FROM native-runtime AS native-builder

ARG DEBIAN_FRONTEND=noninteractive

COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    git \
    libglib2.0-dev \
    libgstreamer-plugins-bad1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    libgstreamer1.0-dev \
    libssl-dev \
    libvips-dev \
    && rm -rf /var/lib/apt/lists/*

#
# server-builder
#

FROM native-builder AS server-builder

WORKDIR /build

COPY go.* ./
RUN go mod download && go mod verify

# Copy application code only after the module layer has been populated.
COPY backend/ /build/backend
COPY docs/ /build/docs
COPY *.go ./

ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64 GOFLAGS=-mod=readonly GOTOOLCHAIN=local
RUN --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go build -trimpath -ldflags "-linkmode 'external' -extldflags '-lstdc++ -lssl -lcrypto' -s -w" -buildvcs=false -o xivi .

#
# development container
#

FROM native-builder AS development

ARG DEBIAN_FRONTEND=noninteractive

COPY --from=node-toolchain /usr/local /usr/local

ENV GOPATH="/home/ubuntu/go" \
    PATH="/home/ubuntu/go/bin:${PATH}"

RUN apt-get update && apt-get install -y --no-install-recommends \
    locales \
    openssh-client \
    procps \
    sudo \
    && usermod -aG sudo ubuntu \
    && echo "ubuntu ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/ubuntu \
    && chmod 0440 /etc/sudoers.d/ubuntu \
    && rm -rf /var/lib/apt/lists/*

USER ubuntu

#
# security verification
#

# Dependencies and pinned analysis tools live in their own stage so GitHub's
# external BuildKit cache can restore them without caching security results.
FROM development AS security-dependencies

WORKDIR /workspace

COPY --chown=ubuntu:ubuntu .npmrc package.json package-lock.json ./
RUN --mount=type=cache,target=/home/ubuntu/.npm,uid=1000,gid=1000,sharing=locked \
    npm ci --ignore-scripts

COPY --chown=ubuntu:ubuntu go.mod go.sum ./
RUN go mod download && go mod verify
RUN go install golang.org/x/vuln/cmd/govulncheck@v1.1.4 \
    && go install github.com/securego/gosec/v2/cmd/gosec@v2.22.11

FROM security-dependencies AS security-tests

COPY --chown=ubuntu:ubuntu . ./

# The workflow marks this stage as no-cache so audits and tests always run.
# Dependency downloads and pinned tool installation remain cached above.
RUN --mount=type=cache,target=/home/ubuntu/.npm,uid=1000,gid=1000,sharing=locked \
    npm audit --audit-level=low \
    && npm run check:license \
    && npm run check \
    && npm run build
RUN --mount=type=cache,target=/home/ubuntu/.cache/go-build,uid=1000,gid=1000,sharing=locked \
    packages="$(go list ./... | grep -v '/node_modules/')" \
    && go test ${packages} \
    && govulncheck ${packages} \
    && gosec -include=G201,G202 . ./backend/... ./docs ./tests/...

#
# deployment
#

FROM native-runtime AS deployment

LABEL org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.source="https://github.com/burmjeff/xivi"

# environment variables
ENV APP_VERSION="1.0" \
    XDG_CACHE_HOME="/xivi/serve/cache" \
    SERVER_HOST="0.0.0.0" \
    SERVER_PORT=3000 \
    SERVER_READ_TIMEOUT=60 \
    XIVI_PRODUCTION="true"

RUN rm -f /usr/bin/pebble \
    && groupadd --system --gid 10001 xivi \
    && useradd --system --uid 10001 --gid xivi --home-dir /xivi --shell /usr/sbin/nologin xivi \
    && mkdir -p /xivi/config /xivi/serve \
    && chown -R xivi:xivi /xivi
WORKDIR /xivi
COPY --chown=xivi:xivi LICENSE NOTICE LICENSING.md THIRD_PARTY_NOTICES.md /xivi/

COPY --chown=xivi:xivi --from=app-builder /app/build /xivi/build
COPY --chown=xivi:xivi --from=server-builder ["/build/xivi", "/xivi/"]
COPY --chown=xivi:xivi backend/platform/database/migrations/ /xivi/database_migrations
COPY --chown=xivi:xivi xivi_channel.png /xivi/xivi_channel.png

VOLUME /xivi/config /xivi/serve

EXPOSE $SERVER_PORT
EXPOSE 65001/udp

USER 10001:10001
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 CMD ["/xivi/xivi", "healthcheck"]
ENTRYPOINT ["/xivi/xivi"]
