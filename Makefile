APP_NAME=discord-bot
build:
	@go build -o bin/$(APP_NAME) main.go

run: build
	./bin/$(APP_NAME)

start-docker-infra: ### Up docker-compose
	docker compose up -d
.PHONY: start-docker-infra

stop-docker-infra: ### Down docker-compose
	docker compose down --remove-orphans
.PHONY: stop-docker-infra