.PHONY: backend-test backend-run backend-build frontend-dev frontend-build up down

backend-test:
	cd backend && go test ./...

backend-run:
	cd backend && go run ./cmd/server

backend-build:
	cd backend && go build -o bin/server ./cmd/server

frontend-dev:
	cd frontend && npm install && npm run dev

frontend-build:
	cd frontend && npm install && npm run build

up:
	docker compose up --build

down:
	docker compose down -v
