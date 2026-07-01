FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server ./cmd/server

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 10001 app
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/server .
RUN mkdir -p /data/images /data/temp && \
    chown -R 10001:10001 /app /data
USER 10001

EXPOSE 8080
ENTRYPOINT ["./server"]