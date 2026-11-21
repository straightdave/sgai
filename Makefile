.PHONY: build build-web build-go dev dev-api dev-web test clean

build: build-web build-go

build-web:
	cd web && npm run build

build-go:
	go build -o bin/server ./cmd/server

dev:
	@-trap 'wait $$api_pid $$web_pid 2>/dev/null; exit 0' INT; \
	trap 'kill $$api_pid $$web_pid 2>/dev/null; wait $$api_pid $$web_pid 2>/dev/null; exit 0' TERM; \
	go run ./cmd/server & api_pid=$$!; \
	(cd web && npm run dev) & web_pid=$$!; \
	wait $$api_pid $$web_pid 2>/dev/null

dev-api:
	go run ./cmd/server

dev-web:
	cd web && npm run dev

test:
	go test -v ./...

clean:
	rm -rf bin web/dist
