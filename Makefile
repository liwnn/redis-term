
run:
	go run ./cmd/redis-term

web:
	go run ./cmd/redis-term-web -addr :9898

build:
	go build -o redis-term ./cmd/redis-term
	go build -o redis-term-web ./cmd/redis-term-web

.PHONY: web
