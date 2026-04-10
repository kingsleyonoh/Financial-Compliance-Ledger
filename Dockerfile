# Build stage
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server cmd/server/main.go

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates wkhtmltopdf
WORKDIR /app
COPY --from=builder /server .
COPY internal/report/templates/ ./internal/report/templates/
COPY migrations/ ./migrations/
EXPOSE 8080
CMD ["./server"]
