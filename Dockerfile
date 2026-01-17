FROM --platform=$BUILDPLATFORM golang:1.25.5-alpine AS builder
WORKDIR /app

ARG TARGETOS
ARG TARGETARCH

RUN mkdir -p /app/logs && \
    addgroup -S proxygroup && adduser -S proxyuser -G proxygroup

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o hls-proxy .

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /app/hls-proxy /hls-proxy
COPY --from=builder --chown=proxyuser:proxygroup /app/logs /app/logs

USER proxyuser
EXPOSE 8080
ENTRYPOINT ["/hls-proxy"]
