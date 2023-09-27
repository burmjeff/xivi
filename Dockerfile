#
# svelte-builder
#

FROM node:20.5.1-alpine3.17 as app-builder

WORKDIR /app
COPY . /app

RUN npm install --package-lock-only
RUN npm prune
RUN npx vite build

#
# server-builder
#

FROM golang:1.21.0-alpine3.17 as server-builder

RUN echo "@main https://dl-cdn.alpinelinux.org/alpine/edge/main" >> /etc/apk/repositories
RUN echo "@community https://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories

RUN apk add build-base
RUN apk add --no-cache vips-dev@community=8.13.3-r1 \
glib-dev=2.74.7-r0 \
gstreamer-dev=1.20.6-r0 \
gst-plugins-base-dev=1.20.6-r0 \
gst-plugins-good=1.20.6-r0 \
gst-plugins-bad-dev=1.20.6-r0 \
gst-plugins-ugly=1.20.6-r0 


WORKDIR /build

COPY backend/ /build/backend
COPY go.* .
COPY *.go .
RUN go mod download

ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64
RUN go install github.com/swaggo/swag/cmd/swag@latest \
    && swag init
RUN go mod tidy
RUN go build -ldflags="-s -w" -buildvcs=false -mod=readonly -v -o apiserver .

#
# deploy
#

FROM alpine:3.17.5 as deployment

RUN echo "@community https://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories

RUN apk add vips@community=8.13.3-r1 \
glib=2.74.7-r0 \
gstreamer=1.20.6-r0 \
gst-plugins-base=1.20.6-r0 \
gst-plugins-good=1.20.6-r0 \
gst-plugins-bad=1.20.6-r0 \
gst-plugins-ugly=1.20.6-r0 

# environment variables
ENV APP_NAME="Xivi" \
APP_VERSION="1.0" \
SERVER_HOST="127.0.0.1" \
SERVER_PORT=8080 \
CONFIG_PATH="/configs" \
SERVE_PATH="/serve" \
MODEL_PATH="/models" \
MODEL_NAME="sentence-transformers/all-MiniLM-L6-v2" \
SERVER_READ_TIMEOUT=60 \
TZ="America/New_York" \
JWT_SECRET_KEY="secret" \
JWT_SECRET_KEY_EXPIRE_MINUTES_COUNT=15

RUN mkdir -p /app
RUN mkdir -p $CONFIG_PATH
RUN mkdir -p $SERVE_PATH
RUN mkdir -p $MODEL_PATH

WORKDIR /app

COPY --from=app-builder /app/build /app/build
COPY --from=server-builder ["/build/apiserver", "/app/"]
COPY database_migrations/ /app/database_migrations

VOLUME $CONFIG_PATH $SERVE_PATH $MODEL_PATH

EXPOSE $SERVER_PORT

ENTRYPOINT ["/app/apiserver"]