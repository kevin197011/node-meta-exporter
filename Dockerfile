FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /node-meta-exporter \
    ./cmd/node-meta-exporter/

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /node-meta-exporter /usr/local/bin/node-meta-exporter

EXPOSE 9101

USER nobody:nobody

ENTRYPOINT ["node-meta-exporter"]
