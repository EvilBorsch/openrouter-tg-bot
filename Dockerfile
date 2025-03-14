# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install git and dependencies
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o telegram-bot .

# Runtime stage
FROM alpine:latest

WORKDIR /main

# Add ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Create volume for persistent data
VOLUME ./data

## Copy binary from builder stage
COPY --from=builder /app/telegram-bot .

## Run the bot
CMD ["./telegram-bot"]
