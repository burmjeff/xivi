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

WORKDIR /app

COPY backend /app/backend
COPY go.* /app
COPY *.go /app
RUN go mod download

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -ldflags="-s -w" -buildvcs=false -mod=readonly -v -o apiserver .

#
# deploy
#

FROM alpine as deployment

WORKDIR /app

COPY --from=app-builder /app/build /app/build
COPY --from=server-builder ["/build/apiserver", "/build/.env", "/app/"]

EXPOSE 8080

ENTRYPOINT ["/apiserver"]
