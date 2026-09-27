# Build stage
FROM golang:alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /rotating-proxy ./cmd/proxy

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /rotating-proxy /usr/local/bin/rotating-proxy

EXPOSE 3128
ENTRYPOINT ["/usr/local/bin/rotating-proxy"]
CMD ["-listen=:3128", "-proxies=proxyList.txt", "-allowed=allowed_ip.txt"]
