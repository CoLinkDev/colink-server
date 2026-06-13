FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /colink-server ./cmd/server

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /colink-server /usr/local/bin/colink-server

EXPOSE 8080

CMD ["colink-server"]
