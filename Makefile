.PHONY: build run test up down

build:
	go build -o subscription-service .

run: build
	./subscription-service

test:
	go test ./...

up:
	docker compose up --build

down:
	docker compose down -v
