.PHONY: run build deps tidy

run:
	ARTHUR_API_URL=http://localhost:8420 \
	JAEGER_API_URL=http://localhost:16686 \
	PORT=8090 \
	go run ./cmd/server

build:
	go build -o bin/observe-platform ./cmd/server

deps:
	go mod tidy && go mod download

tidy:
	go mod tidy
