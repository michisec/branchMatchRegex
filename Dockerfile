# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

FROM golang:1.27.4@sha256:fe517acaa580cc7915457080f850c37e618b3c1e750f98b58fc6429b605147a1 AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY assets ./assets
COPY internal ./internal

RUN go build -trimpath -ldflags="-s -w" -o /out/branchmatchregex .

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/branchmatchregex /branchmatchregex

ENTRYPOINT ["/branchmatchregex"]
