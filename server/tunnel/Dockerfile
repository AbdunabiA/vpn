FROM golang:1.24-alpine AS builder

WORKDIR /build

# xray-core needs git for fetching dependencies
RUN apk add --no-cache git ca-certificates

# Copy go.mod, tidy to resolve all xray-core transitive deps, then download
COPY go.mod ./
COPY . .
RUN go mod tidy && go mod download

# Build the tunnel server binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tunnel-server ./cmd/tunnel

# --- Runtime image ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates iptables ip6tables iproute2 wireguard-tools

WORKDIR /app

COPY --from=builder /build/tunnel-server .
COPY config.example.json config.json

EXPOSE 443 8080

ENTRYPOINT ["./tunnel-server"]
CMD ["-config", "config.json"]
