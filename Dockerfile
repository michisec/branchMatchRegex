# syntax=docker/dockerfile:1

# Build stage: compile a statically linked binary.
FROM golang:1.25 AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY assets ./assets

RUN go build -trimpath -ldflags="-s -w" -o /out/branchmatchregex .

# Runtime stage: minimal image containing only the binary and CA certificates
# (needed when patterns are fetched from an https URL).
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/branchmatchregex /branchmatchregex

ENTRYPOINT ["/branchmatchregex"]
