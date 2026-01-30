# ---------- Build stage ----------
FROM golang:1.24-alpine AS build
WORKDIR /src

# Install certs + git (needed for HTTPS + modules)
RUN apk add --no-cache ca-certificates git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
# IMPORTANT: change the path if your main package is elsewhere
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/nginx-logs-enricher ./cmd/nginx-logs-enricher

# ---------- Runtime stage ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates

COPY --from=build /out/nginx-logs-enricher /usr/local/bin/nginx-logs-enricher

ENTRYPOINT ["nginx-logs-enricher"]
CMD ["--help"]
