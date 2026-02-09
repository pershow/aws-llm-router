# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build -trimpath -ldflags="-s -w" -o /out/aws-cursor-router ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S app \
  && adduser -S -G app -u 10001 app

WORKDIR /app

COPY --from=builder /out/aws-cursor-router /app/aws-cursor-router

RUN mkdir -p /app/data && chown -R app:app /app

USER app

ENV LISTEN_ADDR=:8080
ENV DB_PATH=/app/data/router.db

VOLUME ["/app/data"]

EXPOSE 8080

ENTRYPOINT ["/app/aws-cursor-router"]
