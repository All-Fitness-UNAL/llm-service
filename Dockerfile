# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates tzdata

# Cache go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and prompt templates
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/worker \
    ./cmd/worker

# Runtime stage
FROM alpine:3.20

WORKDIR /app

# Install SSL certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copy compiled binary and templates
COPY --from=builder /app/worker /app/worker
COPY --from=builder /app/prompts /app/prompts

# Set permissions
RUN chown -R appuser:appgroup /app
USER appuser

# Set environment defaults
ENV APP_ENV=production \
    WORKER_COUNT=5 \
    REDIS_ADDR=redis:6379 \
    REDIS_TASK_QUEUE=allfitness:tasks:queue \
    REDIS_RESULT_PREFIX=allfitness:results: \
    REDIS_RESULT_TTL=15m \
    OPENROUTER_PRIMARY_MODEL=anthropic/claude-3.5-sonnet \
    OPENROUTER_FALLBACK_MODEL=openai/gpt-4o-mini \
    PROMPTS_DIR=/app/prompts

ENTRYPOINT ["/app/worker"]
