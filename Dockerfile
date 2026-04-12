# Build stage
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /server cmd/server/main.go

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates wkhtmltopdf
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /server .
COPY internal/report/templates/ ./internal/report/templates/
COPY migrations/ ./migrations/
RUN mkdir -p data/reports && chown -R appuser:appgroup data /app/migrations
USER appuser
EXPOSE 8080
CMD ["./server"]
