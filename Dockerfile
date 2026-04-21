# syntax=docker/dockerfile:1.7

# Stage 1: build the React SPA
FROM node:20-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: build the Go server
FROM golang:1.25-alpine AS api
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/openrow ./cmd/server

# Stage 3: runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -g 10001 -S openrow \
 && adduser -u 10001 -S openrow -G openrow

WORKDIR /app
COPY --from=api /out/openrow /app/openrow
COPY --from=web /web/dist /app/web/dist

ENV SPA_DIR=/app/web/dist \
    HTTP_ADDR=:8080 \
    LOG_LEVEL=info

USER openrow
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD wget --spider -q http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/openrow"]
