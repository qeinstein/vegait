# Stage 1: Build the React dashboard
FROM node:20-alpine AS dashboard-builder

WORKDIR /app/dashboard
COPY dashboard/package.json dashboard/package-lock.json* ./
# Use `npm ci` for a clean, reproducible install from the lockfile.
RUN npm ci
COPY dashboard/ .
RUN npm run build

# Stage 2: Build the Go rate limiter binary
FROM golang:1.21-alpine AS go-builder

RUN apk add --no-cache git

WORKDIR /app
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ .
# Build the gateway service and the load generator (shipped for `loadtest.sh`).
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /rate-limiter ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /loadgen ./cmd/loadgen

# Stage 3: Production runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata curl

WORKDIR /app

COPY --from=go-builder /rate-limiter .
COPY --from=go-builder /loadgen .
COPY --from=dashboard-builder /app/dashboard/dist ./dashboard/dist

EXPOSE 8080 8081

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s \
    CMD curl -f http://localhost:8081/health || exit 1

ENTRYPOINT ["./rate-limiter"]
