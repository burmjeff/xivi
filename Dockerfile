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
# rust_lib-builder
#

FROM rust:slim-bookworm AS lib-builder
VOLUME /usr/src
WORKDIR /usr/src/

RUN apt-get update && apt-get install -y \
libssl-dev \
g++

RUN rustup target add x86_64-unknown-linux-gnu

RUN USER=root cargo new sentence-embed
COPY Cargo.toml Cargo.lock ./
COPY lib/ /usr/src/lib

RUN cargo build --release --target=x86_64-unknown-linux-gnu

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
COPY --from=lib-builder /usr/src/target/x86_64-unknown-linux-gnu/release/libcandle_embeddings.a /usr/src/target/candle-embeddings.h ./lib_build/
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
gstreamer1.0-plugins-ugly

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
