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
COPY package.json package-lock.json ./
RUN npm ci
COPY . /app
RUN npm run build

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

COPY backend/ /build/backend
COPY docs/ /build/docs
COPY go.* ./
COPY *.go ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64 GOFLAGS=-mod=readonly GOTOOLCHAIN=local
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
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
# deployment
#

FROM native-runtime AS deployment

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
