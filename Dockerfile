# Use the BUILDPLATFORM to run the compiler on the native GitHub runner (AMD64)
FROM --platform=$BUILDPLATFORM golang:1.25.5-alpine AS builder
WORKDIR /app

# These ARG values are automatically filled by Docker Buildx
ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
RUN go mod download
COPY . .

# Use TARGETOS and TARGETARCH to cross-compile for the VPS
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o hls-proxy .

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/hls-proxy /hls-proxy

EXPOSE 8080
ENTRYPOINT ["/hls-proxy"]
