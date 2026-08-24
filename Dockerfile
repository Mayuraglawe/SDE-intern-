# Multi-stage Dockerfile for Go Order Position Engine
# Stage 1: Build binaries
FROM golang:alpine AS builder
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/position_service ./cmd/position_service
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/order_service ./cmd/order_service

# Stage 2: Runtime image for Position Maintaining Service & Web Dashboard
FROM alpine:latest AS position-service
WORKDIR /app

COPY --from=builder /app/bin/position_service /app/position_service
COPY --from=builder /app/web /app/web

EXPOSE 8080
ENTRYPOINT ["/app/position_service", "--port=8080", "--web-dir=/app/web"]
