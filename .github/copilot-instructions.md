# Copilot instructions for `relayhat`

## Build and test commands

Use the `Makefile` targets for release-style builds:

```bash
make build_arm64      # Raspberry Pi 3/4/5/Zero2, 64-bit
make build_arm6       # Raspberry Pi Zero / Pi 1, 32-bit ARMv6
make build_arm7       # Raspberry Pi 2/3/4/5, 32-bit ARMv7
make build_arm8       # Raspberry Pi 3/4/5/Zero2, 32-bit ARMv8
make build_linux64
make build_mac_arm64
make deploy           # builds arm64 and copies the binary to the configured Pi host
make deploy_dev       # like deploy, but includes Swagger UI via the `swagger` build tag
```

General Go commands:

```bash
go test ./...
go test ./path/to/package -run TestName
go build ./cmd/main.go
```

Swagger docs are generated from handler annotations:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
docs/generate.sh
```

`go test ./...` currently does not build cleanly on this macOS environment because the GPIO dependency chain is Linux/Raspberry-Pi specific. Prefer validating on a Pi/Linux target when changes touch relay or GPIO integration.

## High-level architecture

`cmd/main.go` is the lifecycle entrypoint. It parses flags, resolves the config path (`-config` or `CONFIG_FILE`), loads YAML config, initializes logging with `github.com/womat/golib/xlog`, and then runs a restartable main loop. That loop recreates the app after `SIGHUP`, so config reload is a first-class behavior rather than a one-time startup path.

`app/app.go` wires the runtime together. `App.Init()` recreates the in-memory relay map from `config.Relays`, creates one GPIO-backed relay instance per configured relay, and only then calls `SetupRoutes()`. `App.Run()` starts signal handling and the HTTPS server. Shutdown and reload both go through `shutdownProcedure()`, which cancels context, waits for the web server goroutines, closes relay resources, and then signals either restart or final shutdown.

`app/routes.go` defines the HTTP surface using Go's method-based `http.ServeMux` patterns. `/version` is public. `/health`, `/relays`, `/relays/{name}`, and `/relays/{name}/{state}` are wrapped with `web.WithAuth(...)`, so the normal auth path is centralized middleware rather than per-handler checks. Global middleware order is CORS, then IP filtering, then request logging.

The relay control path is `app/api_relay.go` -> `app.relayGet` / `app.relaySet` -> `pkg/relay/relay.go`. The handler layer is thin: it extracts path params with `r.PathValue(...)`, delegates to internal methods, and serializes responses with `web.Encode(...)`. GPIO interaction is isolated in `pkg/relay`, which wraps the underlying pin implementation and exposes `TurnOn`, `TurnOff`, `Toggle`, and `GetState`.

TLS and environment behavior live in `app/webservices.go` and `app/config.go`. In production, the server loads certificate files from config. In development, if the configured cert files do not exist, the app falls back to embedded self-signed certs. Swagger is optional and controlled by the `swagger` build tag: `app/swagger.go` registers `/swagger/`, while `app/swagger_stub.go` compiles to a no-op when the tag is absent.

## Key conventions

- Relay definitions are config-driven. Add or remove relays through `config/config.yaml` and the `Config.Relays` map; `App.Init()` rebuilds the runtime relay map on each reload so removed relays do not survive a `SIGHUP`.
- `app.relays` is shared state protected by `app.mu`. Keep reads under `RLock`/`RUnlock` and writes under `Lock`/`Unlock`. `HandleRelayGetAll()` intentionally snapshots relay names, sorts them, and then resolves them one by one for stable output.
- Config struct fields in `app/config.go` intentionally use exported CamelCase names because the code is prepared for future overwrite/CLI-based field overrides. Preserve that naming pattern when extending config.
- API handlers follow a consistent split: `Handle...()` returns `http.Handler`, path params come from `r.PathValue(...)`, and shared logic lives in helper methods such as `relayGet` / `relaySet` instead of being embedded directly in the closure.
- The project uses `github.com/womat/golib/web` for auth, CORS, IP filtering, and JSON encoding. Reuse those helpers instead of re-implementing middleware or response formatting locally.
- Swagger docs are generated, not hand-maintained. Route annotations live next to handlers in `app/api_*.go`, `docs/docs.go` is generated output, and `docs/generate.sh` is the repo's regeneration entrypoint.
- Versioning is project-specific: `app.VERSION` is not conventional semver and is maintained as a fixed application-specific string in `app/app.go`. Keep that format if you update it.
