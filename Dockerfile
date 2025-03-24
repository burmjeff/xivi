#
# svelte-builder
#

FROM node:lts-bookworm-slim AS app-builder

WORKDIR /app
COPY . /app

RUN npm install --package-lock-only
RUN npm prune
RUN npx vite build

#
# server-builder
#

FROM golang:1.23-bookworm AS server-builder

RUN apt-get update && apt-get install -y \
build-essential \
libvips-dev \
libglib2.0-dev \
libgstreamer1.0-dev \
libgstreamer-plugins-base1.0-dev \
gstreamer1.0-plugins-good \
libgstreamer-plugins-bad1.0-dev \
gstreamer1.0-plugins-ugly \
libssl-dev

WORKDIR /build

COPY backend/ /build/backend
COPY go.* .
COPY *.go .
RUN go mod download

ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64
RUN go install github.com/swaggo/swag/cmd/swag@latest \
    && swag init
RUN go mod tidy
RUN go build -ldflags "-linkmode 'external' -extldflags '-lstdc++ -lssl -lcrypto' -s -w" -buildvcs=false -mod=readonly -v -o xivi .

#
# deploy
#

FROM ubuntu:noble AS deployment

RUN apt-get update && apt-get install -y --no-install-recommends \
openssl \
ca-certificates \
tzdata \
libvips42 \
libglib2.0-0 \
libgstreamer1.0-0 \
gstreamer1.0-plugins-base \
gstreamer1.0-plugins-good \
gstreamer1.0-plugins-bad \
gstreamer1.0-plugins-ugly \
curl

# Install ONNX Runtime
RUN curl -L https://github.com/microsoft/onnxruntime/releases/download/v1.21.0/onnxruntime-linux-x64-1.21.0.tgz -o /tmp/onnxruntime.tgz && \
    mkdir -p /usr/local/onnxruntime && \
    tar -xzf /tmp/onnxruntime.tgz -C /usr/local/onnxruntime --strip-components=1 && \
    rm /tmp/onnxruntime.tgz

# Set up ONNX Runtime environment variables
ENV LD_LIBRARY_PATH="/usr/local/onnxruntime/lib:${LD_LIBRARY_PATH}" \
    ONNX_PATH="/usr/local/onnxruntime/lib/libonnxruntime.so"

# environment variables
ENV APP_NAME="Xivi" \
APP_VERSION="1.0" \
TZ="America/New_York" \
SERVER_HOST="127.0.0.1" \
SERVER_PORT=3000 \
SERVER_READ_TIMEOUT=60 \
JWT_SECRET_KEY="secret" \
JWT_SECRET_KEY_EXPIRE_MINUTES_COUNT=15 \
LOG_LEVEL=3

RUN mkdir -p /xivi
WORKDIR /xivi

COPY --from=app-builder /app/build /xivi/build
COPY --from=server-builder ["/build/xivi", "/xivi/"]
COPY backend/platform/database/migrations/ /xivi/database_migrations
COPY xivi_channel.png /xivi/xivi_channel.png

VOLUME /xivi/config /xivi/serve

EXPOSE $SERVER_PORT

ENTRYPOINT ["/xivi/xivi"]
