# syntax=docker/dockerfile:1

FROM golang:1.25 AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY assets ./assets

RUN go build -trimpath -ldflags="-s -w" -o /out/branchmatchregex .

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/branchmatchregex /branchmatchregex

ENTRYPOINT ["/branchmatchregex"]
