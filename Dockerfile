# Multi-stage build for minimal image size
FROM golang:1.25.1-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -trimpath -o server ./server

# Final stage
FROM alpine:3.22.2

# Create non-root user and set permissions in one layer
RUN adduser -D appuser && mkdir /app && chown appuser:appuser /app

# Copy the pre-built binary
COPY --from=builder /app/server /app/

# Set working directory
WORKDIR /app

# Use non-root user
USER appuser

# Expose port (server runs on 8080)
EXPOSE 8080

# Run the application
CMD ["/app/server"]