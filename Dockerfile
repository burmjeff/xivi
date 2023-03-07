#
# svelte-builder
#

FROM node:alpine as app-builder

WORKDIR /app
COPY . /app

RUN npm install --package-lock-only
RUN npm prune
RUN npx vite build

#
# server-builder
#

FROM golang:alpine as server-builder

RUN apk add build-base

WORKDIR /build

COPY backend/ /build/backend
COPY go.* .
COPY *.go .
RUN go mod download

ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64
RUN go install github.com/swaggo/swag/cmd/swag@latest \
    && swag init \
    && go mod tidy \
    && go build -ldflags="-s -w" -buildvcs=false -mod=readonly -v -o apiserver .

#
# deploy
#

FROM alpine as deployment

# environment variables
ENV APP_NAME="Xivi" \
APP_VERSION="1.0" \
SERVER_HOST="0.0.0.0" \
SERVER_PORT=8080 \
CONFIG_PATH="/configs" \
M3U_FILEPATH="/configs/stream" \
EPG_FILEPATH="/configs/stream" \
MODEL_PATH="/models" \
MODEL_NAME="sentence-transformers/all-MiniLM-L6-v2" \
SERVER_READ_TIMEOUT=60 \
TZ="America/New_York" \
JWT_SECRET_KEY="secret" \
JWT_SECRET_KEY_EXPIRE_MINUTES_COUNT=15

RUN mkdir -p /app
RUN mkdir -p $CONFIG_PATH
RUN mkdir -p $MODEL_PATH

WORKDIR /app

COPY --from=app-builder /app/build /app/build
COPY --from=server-builder ["/build/apiserver", "/app/"]
COPY database_migrations/ /app/database_migrations

VOLUME $CONFIG_PATH $M3U_FILEPATH $EPG_FILEPATH $MODEL_PATH

EXPOSE $SERVER_PORT

ENTRYPOINT ["/app/apiserver"]