#
# svelte-builder
#

FROM node:20.7.0-alpine3.18 as app-builder

WORKDIR /app
COPY . /app

RUN npm install --package-lock-only
RUN npm prune
RUN npx vite build

#
# server-builder
#

FROM golang:1.21.1-alpine3.18 as server-builder

RUN echo "@main https://dl-cdn.alpinelinux.org/alpine/edge/main" >> /etc/apk/repositories
RUN echo "@community https://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories

RUN apk add build-base
RUN apk add --no-cache vips-dev@community=8.14.3-r0 \
glib-dev \
gstreamer-dev \
gst-plugins-base-dev \
gst-plugins-good \
gst-plugins-bad-dev \
gst-plugins-ugly


WORKDIR /build

COPY backend/ /build/backend
COPY go.* .
COPY *.go .
RUN go mod download

ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64
RUN go install github.com/swaggo/swag/cmd/swag@latest \
    && swag init
RUN go mod tidy
RUN go build -ldflags="-s -w" -buildvcs=false -mod=readonly -v -o xivi .

#
# deploy
#

FROM alpine:3.18.4 as deployment

RUN echo "@community https://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories

RUN apk add vips@community=8.14.3-r0 \
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
SERVER_HOST="0.0.0.0" \
SERVER_PORT=8080 \
SERVER_READ_TIMEOUT=60 \
MODEL_NAME="sentence-transformers/all-MiniLM-L6-v2" \
JWT_SECRET_KEY="secret" \
JWT_SECRET_KEY_EXPIRE_MINUTES_COUNT=15 \
LOG_LEVEL=3

RUN mkdir -p /xivi
WORKDIR /xivi

COPY --from=app-builder /app/build /xivi/build
COPY --from=server-builder ["/build/xivi", "/xivi/"]
COPY backend/platform/database/migrations/ /xivi/database_migrations

VOLUME /config /serve

EXPOSE $SERVER_PORT

ENTRYPOINT ["/xivi/xivi"]