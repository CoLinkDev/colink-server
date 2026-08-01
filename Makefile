APP=colink-server

.PHONY: run test tidy fmt docker-up docker-down frontend-install frontend-dev frontend-test frontend-build

run:
	go run ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

docker-up:
	docker compose -f docker-compose.dev.yml up -d --build

docker-down:
	docker compose -f docker-compose.dev.yml down

frontend-install:
	pnpm --dir web/console install --frozen-lockfile

frontend-dev:
	pnpm --dir web/console run dev

frontend-test:
	pnpm --dir web/console run test

frontend-build:
	pnpm --dir web/console run build
