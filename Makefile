.PHONY: build test vet lint run compose-up compose-down clean

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# Run locally with the in-memory store (no Redis / Docker required).
run:
	go run ./cmd/gateway

# Full stack: Traefik edge + Redis + the gateway.
compose-up:
	docker compose -f deploy/docker-compose.yml up --build

compose-down:
	docker compose -f deploy/docker-compose.yml down

clean:
	rm -f gateway