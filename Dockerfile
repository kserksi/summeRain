FROM --platform=$BUILDPLATFORM node:24.18.0-alpine AS frontend-builder

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine AS backend-builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.24.1

ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown

LABEL org.opencontainers.image.title="summeRain" \
      org.opencontainers.image.description="Self-hosted image hosting and gallery service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}" \
      org.opencontainers.image.licenses="Apache-2.0"

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S -g 10001 app && \
    adduser -S -D -H -u 10001 -G app app && \
    mkdir -p /app/web /data/images/.staging && \
    chown -R 10001:10001 /app /data

ENV TZ=Asia/Shanghai \
    STORAGE_PATH=/data/images \
    TEMP_PATH=/data/images/.staging
WORKDIR /app

COPY --from=backend-builder /out/server ./server
COPY --from=frontend-builder /src/backend/web/ ./web/

USER 10001:10001
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["./server"]
