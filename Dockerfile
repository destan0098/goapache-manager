# GoApache Manager Backend
FROM golang:1.26-bookworm AS builder

WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends
    gcc
    libc6-dev
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build
    -trimpath
    -ldflags="-s -w"
    -o /out/goapache
    ./cmd/server


FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends
    ca-certificates
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/goapache

COPY --from=builder /out/goapache /opt/goapache/goapache

ENV GOAPACHE_DB=/var/lib/goapache/goapache.db

EXPOSE 9090

ENTRYPOINT ["/opt/goapache/goapache"]