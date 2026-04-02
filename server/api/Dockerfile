FROM golang:1.25-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates

# Copy go.mod first, then tidy to generate go.sum and download deps
COPY go.mod ./
RUN go mod tidy && go mod download

# Copy source and build
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o api-server ./cmd

# --- Runtime image ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /build/api-server .
COPY migrations/ ./migrations/

EXPOSE 3000

ENTRYPOINT ["./api-server"]
