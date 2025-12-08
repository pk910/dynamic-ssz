# Build stage - use xx for cross-compilation
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the dynssz-gen binary with cross-compilation
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /app/bin/dynssz-gen ./dynssz-gen

# Final stage
FROM alpine:latest

# Create a non-root user
RUN addgroup -g 1000 dynssz && \
    adduser -D -s /bin/sh -u 1000 -G dynssz dynssz

# Set working directory
WORKDIR /data

# Copy the binary from builder stage
COPY --from=builder /app/bin/dynssz-gen /usr/local/bin/dynssz-gen

# Change ownership and make executable
RUN chmod +x /usr/local/bin/dynssz-gen

# Switch to non-root user
USER dynssz

# Set entrypoint
ENTRYPOINT ["dynssz-gen"]
CMD ["--help"]
