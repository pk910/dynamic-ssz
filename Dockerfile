# Build stage - use xx for cross-compilation
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_COMMIT=""
ARG BUILD_TIME=""

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the dynssz-gen binary with cross-compilation
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X github.com/pk910/dynamic-ssz/codegen.BuildCommit=${BUILD_COMMIT} -X github.com/pk910/dynamic-ssz/codegen.BuildTime=${BUILD_TIME}" \
    -o /app/bin/dynssz-gen ./dynssz-gen

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
