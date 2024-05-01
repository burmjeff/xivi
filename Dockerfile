#
# svelte-builder
#

FROM node:21.6.0-alpine3.19 as app-builder

WORKDIR /app
COPY . /app

RUN npm install --package-lock-only
RUN npm prune
RUN npx vite build

#
# rust_lib-builder
#

FROM rust:1.77.2-alpine3.19 AS lib-builder
VOLUME /usr/src
WORKDIR /usr/src/

RUN apk add musl-dev openssl-dev g++
RUN rustup target add x86_64-unknown-linux-musl

RUN USER=root cargo new sentence-embed
COPY Cargo.toml Cargo.lock ./
COPY lib/ /usr/src/lib

RUN cargo build --release --target=x86_64-unknown-linux-musl --features "musl-libc"

#
# server-builder
#

FROM golang:1.22.2-alpine3.19 as server-builder

RUN echo "@main https://dl-cdn.alpinelinux.org/alpine/edge/main" >> /etc/apk/repositories
RUN echo "@community https://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories

RUN apk add build-base
RUN apk add --no-cache vips-dev@community=8.15.0-r0 \
glib-dev \
gstreamer-dev \
gst-plugins-base-dev \
gst-plugins-good \
gst-plugins-bad-dev \
gst-plugins-ugly \
musl-dev \
openssl-dev

WORKDIR /build

COPY backend/ /build/backend
COPY go.* .
COPY *.go .
COPY --from=lib-builder /usr/src/target/x86_64-unknown-linux-musl/release/libcandle_embeddings.a /usr/src/target/candle-embeddings.h /lib_build/
RUN go mod download

ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64
RUN go install github.com/swaggo/swag/cmd/swag@latest \
    && swag init
RUN go mod tidy
RUN go build -ldflags "-linkmode 'external' -extldflags '-lstdc++ -lssl -lcrypto' -s -w" -buildvcs=false -mod=readonly -v -o xivi .

#
# deploy
#

FROM alpine:3.19.1 as deployment

RUN echo "@community https://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories

RUN apk add tzdata \
vips@community=8.15.0-r0 \
glib \
gstreamer \
gst-plugins-base \
gst-plugins-good \
gst-plugins-bad \
gst-plugins-ugly 

# environment variables
ENV APP_NAME="Xivi" \
APP_VERSION="1.0" \
TZ="America/New_York" \
SERVER_HOST="127.0.0.1" \
SERVER_PORT=8080 \
SERVER_READ_TIMEOUT=60 \
JWT_SECRET_KEY="secret" \
JWT_SECRET_KEY_EXPIRE_MINUTES_COUNT=15 \
LOG_LEVEL=3

RUN mkdir -p /xivi
WORKDIR /xivi

COPY --from=app-builder /app/build /xivi/build
#COPY --from=lib-builder /usr/src/target/x86_64-unknown-linux-musl/release/libcandle_embeddings.so /usr/lib/
COPY --from=server-builder ["/build/xivi", "/xivi/"]
COPY backend/platform/database/migrations/ /xivi/database_migrations
COPY xivi_channel.png /xivi/xivi_channel.png

VOLUME /config /serve

EXPOSE $SERVER_PORT

ENTRYPOINT ["/xivi/xivi"]