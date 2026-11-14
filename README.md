# sgai

Go web service using [Chi](https://github.com/go-chi/chi) with an embedded React JS frontend (Vite).

## Structure

- `embed.go` — Embeds `web/dist` into the Go binary
- `cmd/server/main.go` — HTTP server entrypoint with graceful shutdown
- `internal/server/server.go` — Chi router, `/api/*` endpoints, and SPA static/fallback handler
- `web/` — Pure Vite + React JS application

## Development

Start both the Go API server (`:8080`) and Vite dev server (`:5173`, proxying `/api` to `:8080`) with a single command:

```bash
make dev
```

Then open http://localhost:5173 in your browser.

Or run them in separate terminals:

```bash
# Terminal 1: Go API server on :8080
make dev-api

# Terminal 2: Vite dev server on :5173
make dev-web
```

## Production Build

Builds the React bundle into `web/dist` and embeds it into `bin/server`:

```bash
make build
./bin/server
```

## Tests

```bash
make test
```
