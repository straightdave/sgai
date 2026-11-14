.PHONY: build build-web build-go dev dev-api dev-web test clean

build: build-web build-go

build-web:
	cd web && npm run build

build-go:
	go build -o bin/server ./cmd/server

dev:
	@trap 'kill 0' INT TERM EXIT; \
	$(MAKE) dev-api & \
	$(MAKE) dev-web & \
	wait

dev-api:
	go run ./cmd/server

dev-web:
	cd web && npm run dev

test:
	go test -v ./...

clean:
	rm -rf bin web/dist
