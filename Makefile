.PHONY: build test lint docker docker-up docker-down clean

build:
	go build -o apibudget-server ./cmd/apibudget-server

test:
	go test -race ./...

lint:
	go vet ./...
	golangci-lint run

docker:
	docker build -t apibudget-server .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

clean:
	rm -f apibudget-server
	rm -f coverage.out
