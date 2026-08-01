FROM node:20-alpine AS frontend-builder

WORKDIR /app/web/console

RUN corepack enable && corepack prepare pnpm@10 --activate

COPY web/console/package.json web/console/pnpm-lock.yaml web/console/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/console/ ./
RUN pnpm build

FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /app/web/console/dist/ /app/internal/handler/frontend/dist/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /colink-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /colink-update-server ./cmd/update-server

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /colink-server /usr/local/bin/colink-server
COPY --from=builder /colink-update-server /usr/local/bin/colink-update-server

EXPOSE 8080

CMD ["colink-server"]
