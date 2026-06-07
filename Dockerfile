FROM golang:1.22-bookworm AS builder

WORKDIR /app

# Install Chrome dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Download dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Build the binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo 'none') -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /scrapeowl \
    ./cmd/scrapeowl

# Runtime image
FROM chromedp/headless-shell:latest

WORKDIR /scrapeowl

# Copy the binary and web assets
COPY --from=builder /scrapeowl /usr/local/bin/scrapeowl
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY web ./web
COPY examples ./examples

# Create output directory
RUN mkdir -p /scrapeowl/output

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/stats || exit 1

# Run the server
ENTRYPOINT ["scrapeowl", "serve"]
CMD ["--addr", ":8080", "--db", "/scrapeowl/scrapeowl.db"]
