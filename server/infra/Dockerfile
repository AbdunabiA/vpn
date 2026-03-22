FROM golang:1.22-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o infra-orchestrator ./cmd

# --- Runtime image ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates openssh-client terraform

WORKDIR /app

COPY --from=builder /build/infra-orchestrator .

ENTRYPOINT ["./infra-orchestrator"]
