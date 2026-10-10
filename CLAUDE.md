# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`relayhat` is a Go daemon that switches BC Robotics Relay HATs (2-channel Pi Zero HAT on GPIO 4/17, 4-channel HAT on GPIO 4/17/22/27) through Raspberry Pi GPIO outputs and exposes them via a TLS REST API with API-key auth. Single binary, cross-compiled for the Pi. It shares its skeleton (lifecycle, config, TLS, Makefile, CI, release) with the sibling project `s0meter` — when changing one of those parts, check whether the other project needs the same change.

## Build / develop

**The GPIO dependency (`warthog618/go-gpiocdev` via `womat/golib/gpio/rpi`) only compiles for Linux.** On macOS `go build ./...` and `go vet ./...` fail with `undefined: uapi.*`. Always set the target explicitly when checking code locally:

```sh
GOOS=linux GOARCH=arm64 go vet ./...
GOOS=linux GOARCH=arm64 go build ./...
```

`make lint` runs `go vet`, golangci-lint (v2, default linters; `.golangci.yml` holds the exclusions, each with its reason) and govulncheck for linux/armv6, each also with `-tags swagger` where it applies. The tools are built for the host into `bin/tools` and pinned in the Makefile, because `go run` under the target's `GOOS`/`GOARCH` would build a binary the host cannot execute. Fix a finding unless it is deliberate; then exclude it narrowly (one function in `exclude-functions`, or path + linter + text), never by switching a linter off.

```sh
make build_arm6        # every Pi in 32-bit mode, Pi 1 / Zero — the default deployment target
make build_arm7        # Pi 2/3/4/5, 32-bit OS
make build_arm64       # Pi 3/4/5/400/Zero2, 64-bit OS
make build_arm6_dev    # + Swagger UI (-tags swagger); _dev variants exist per arch
make deploy            # build for $(PI_ARCH) then scp to $(PI_USER)@$(PI_HOST)
make clean
```

`ensure_dev_certs` (a prerequisite of every build target) generates `app/certs/dev_{cert,key}.pem` if missing; these are `//go:embed`-ed and gitignored, so a fresh clone must build via `make`, not bare `go build`.

Tests use golib's in-memory GPIO emulator (`gpio/rpiemu`) through `relay.NewWithPin` and the `App.openRelay` hook, so they need no hardware — but they compile on Linux only, like everything here. Run them with `make test` (`go test -race ./...`), on macOS via `docker run --rm -v "$PWD":/src -w /src golang:1.27 make test`. `VERSION` (`app/app.go`), `buildDate` and `buildCommit` (`cmd/main.go`) are all `var`s injected via `-ldflags` — never edit them in source. The Makefile derives `VERSION` from `git describe --tags`; GoReleaser uses the tag itself.

### Releases

**There is one branch, `main`: work is committed to it and a release is a tag on it.** `make release TAG=vX.Y.Z` refuses to run from any other branch, with a dirty tree, or when `main` and `origin/main` differ; `.github/workflows/release.yml` re-checks that the tagged commit is on `main`, so a hand-made `git tag` cannot bypass it. Use a short-lived feature branch for work that must not land on `main` yet. Because releases are tags on the branch everyone builds from, `git describe` — and so a local build's version — always sees the latest release.

Versioning is SemVer and the Git tag is the single source of truth. `.github/workflows/release.yml` runs `goreleaser release --clean`, which builds linux arm64/armv7/armv6 and publishes a GitHub release with checksums and a grouped changelog. `.goreleaser.yaml`'s `before` hook must keep running `make ensure_dev_certs` (GoReleaser calls `go build` directly), and archives must keep shipping `README.md` (third-party license overview) and `LICENSE` (MIT). When adding a dependency, update the license table in `README.md`.

`.github/workflows/ci.yml` runs on every push/PR against `main`: a `test` job (native, `make test`) and a `build` matrix over armv6/armv7/arm64 that vets, builds (also `-tags swagger`) and runs golangci-lint (also `-tags swagger`) and govulncheck. All actions are pinned to a commit SHA with the release in a comment, `golangci-lint` and `govulncheck` to a version (in `ci.yml` and the Makefile's `lint` target); `.github/dependabot.yml` updates actions and Go modules weekly, but not the `go install` pins.

`PI_USER`/`PI_HOST`/`PI_PATH` default to placeholders; the actual device comes from environment variables (set once for all projects; they win over the `?=` defaults) or, project-specific, from `Makefile.local` (gitignored, pulled in via `-include`). **`PI_ARCH` defaults to `arm6`**, like in s0meter: it runs on every Pi in 32-bit mode, and a Pi Zero (1st gen, 2-channel HAT) is ARMv6 only, where an arm64 binary dies with `Exec format error`. Every `deploy*` target follows it, so none of them may hardcode an architecture; CI runs a matrix over armv6/armv7/arm64. `make deploy` is the development loop (binary reports a `-dirty` version), `make deploy_release TAG=vX.Y.Z` downloads, verifies and copies a published release.

### Screenshots

The README screenshots (`docs/screenshots/web-ui*.png`) and `docs/social-preview.png` are rendered from the real `app/ui/index.html` with a mocked API in headless Chromium. The same script first clicks through the page (login, two-step switch, double tap, `Esc`, switch lock, rejected key) and fails when it misbehaves — run it after every change to the page. The example relays, host names and addresses are made up; keep real ones out of this public repository. After a visible change upload the social preview again under the repository's Settings → Social preview (GitHub has no API for it):

```sh
docker run --rm -v "$PWD":/src -w /src mcr.microsoft.com/playwright/python:v1.52.0-noble \
  sh -c 'pip install -q playwright==1.52.0 && python3 docs/screenshots/capture.py'
```

### Swagger

Swagger UI is behind the `swagger` build tag (`app/swagger.go` vs `app/swagger_stub.go`, both defining `registerSwaggerRoute`). Regenerate `docs/` from the annotations after changing API handlers:

```sh
docs/generate.sh   # must run from the project root; needs swaggo/swag installed
```

## Architecture

Layering is strict: `cmd` → `app` → `pkg/*`. Lower layers never import upward.

- **`cmd/main.go`** — flags, config load/validate, logger (`newLogger`, plain `log/slog`), and the **restart loop**. `run()` subscribes to SIGHUP/SIGTERM/SIGINT **once** and hands that channel to every `App` — never `signal.Stop`/`Reset` it inside `app`. Each iteration builds `app.New(config, signals, checkReload, inherited).Run()`; `checkReload` loads and validates the config file and is called by the SIGHUP handler **before** tearing anything down, so a broken file is refused and the running `App` keeps going. On `a.Restart()` it takes `a.Handover()` as `inherited` for the next `App`; on `a.Shutdown()` it exits. A restart that passes the check but fails in `Run` (port, GPIO) falls back to `lastGood`, the config of the previous `App`: a failed `Run` never closes relays, `Init` and the web-server error path hand them over (`Handover`), and only a failing first start exits, closing them via the deferred close of `inherited`. `App.stop` cancels the context under `app.mu`, which `relaySet` and `resolveClient` hold while they check it, so no request switches a relay and no lookup joins `app.wg` once a shutdown has begun (503 instead). `cmd/README.md` is `//go:embed`-ed as `--help` output.
- **`app/app.go`** — wiring and lifecycle. Owns the `context.Context` that the web server and signal handler are cancelled by. The signal goroutine is the **only** caller of `shutdownProcedure`: SIGHUP → `ModeRestart`, SIGTERM/SIGINT → `ModeStop`, and a web server that stops on its own reports on `app.serverErr` → `ModeRestart` (it must not call `shutdownProcedure` itself, because that waits on `app.wg`, which tracks the server goroutine).
- **Relay handover** — the reason relays keep their state across a reload. golib's `rpi` pin `Close()` reconfigures the line as input, which switches the relay off, so a restart must not close relays that stay configured. `shutdownProcedure(ModeRestart)` moves the open relays (`*app.Relay`: pin, config, last switch) into `app.handover` (keyed by GPIO) before `Cleanup`; the next `App.Init` takes over every relay whose GPIO is still configured (also under a new name), gives it the new `RelayConfig`, keeps its last switch, and closes the rest. If `Init` fails it closes everything it opened or took over; `run()` closes a pending handover when it returns early. A stop closes all relays, i.e. switches them off.
- **Start state** — `RelayConfig.StartState` (`off` default, `on`, `last`) applies to relays `Init` *opens*, i.e. on a cold start and to relays a reload adds; taken-over relays keep their state. `last` reads `stateFile` (`app/state.go`, written atomically). A missing, empty or damaged file is never fatal — those relays start off and the log says why. The file is written once in `Init` (creates it, surfaces permission problems at start) and after every API switch via `saveStates` (under `stateMu`, reading the states inside the lock so the last write wins); never on stop, which would overwrite the last state with "off". It is read and written whenever `stateFile` is set, also without `last`, because it carries the last switch; `stateFile: ""` turns it off.
- **Last switch** — every `Relay` holds a `Change` (time, `source` api|start, client IP, reverse DNS host) behind its own mutex, because it outlives the App on a handover. `relaySet` records it, saves, and resolves the host afterwards in a goroutine (`app.lookupAddr`, 2 s timeout, bound to `app.ctx`, tracked in `app.wg` so a shutdown cancels and waits for it); `setHost` only applies to the switch it was started for. On a cold start a relay keeps the stored change when it starts in the stored state, otherwise the start is its change. The state file is a map `name: {state, changed, source, client, host}`; the 1.7 form `name: on|off` is still read (`savedRelay.UnmarshalYAML`).
- **Switch lock** — `RelayConfig.MinSwitchInterval` refuses a switch within that time of `LastChange` (from any source, so it survives reloads and restarts). `(*Relay).switchTo` reads the state, checks the lock, switches and records the change under the relay's own mutex, so concurrent requests cannot both pass; a request for the current state is not a switch (no new change, no lock, allowed while locked). `relaySet` maps `errSwitchLocked` to 429 and `HandleRelaySet` sets `Retry-After`. `applyStartStates` never checks the lock. Time comes from `app.now` so tests can move the clock.
- **`app/config.go`** — YAML config decoded with `KnownFields(true)` (unknown keys are an error), `${VAR}` — and only that form, never bare `$` — expanded on the raw file, yaml.v3 itself refusing a duration without a unit, `0` included (pinned in `config_durations_test.go`), defaults from `NewConfig()`, a `Validate()` that `cmd` calls before `app.New` (GPIO 2–27, unique per relay), and `Warnings()` for non-fatal findings such as a weak `apiKey`. Authentication is API key only; there is no JWT config.
- **`app/routes.go` / `api_*.go`** — `http.ServeMux` with Go 1.22 method patterns. Middleware chain, outermost first: `WithLogging` → `WithIPFilter` → `WithCORS` → mux. Auth is per-route via `web.WithAuth` (`X-API-Key`, from `womat/golib/web`). `GET /{$}` (the web page, `/` only) and `/version` are public; `/health`, `/relays`, `/relays/{name}` and `PATCH /relays/{name}/{state}` are protected. CORS allows GET, PATCH and OPTIONS only. Handlers return `http.Handler`, take path params via `r.PathValue`, and keep shared logic in `relayGet`/`relaySet`. Errors go through `web.WriteError` (JSON `{"error": …}`, 5xx without internals, logged with method/path/status) — never `web.Encode` of a raw error. `relaySet` logs every switch at info with relay, GPIO, old/new state and client address (audit trail).
- **`app/api_ui.go` / `app/ui/index.html`** — the web page, the same pattern as s0meter's: one `//go:embed`-ed file with inline JS/CSS, strict CSP (`uiCSP`, `connect-src 'self'`), no external resources. It keeps the API key in `localStorage` (`relayhat.apiKey`), polls `relays` and `health` every 3 s with relative URLs, and switches only through `PATCH /relays/{name}/{state}` after a two-step confirmation (arm, then a second tap within 3 s, at least 300 ms apart; `Esc`, a timeout or a state change from elsewhere disarm). Cards are updated in place and only rebuilt when the set of relays changes, so an armed button survives a poll. What a card shows comes from `HTTPResponse.Display` (`label`, `color`, `onText`, `offText` of `RelayConfig`) and `LastChange`; ages are counted from the server's `ageSeconds`, never from the browser clock. The relay names are the API paths and must stay stable for other clients (e.g. Node-RED), which is why display names are separate fields.
- **`app/webservices.go`** — HTTPS only. Falls back to the embedded dev cert when `certFile` does not exist, but only with `env: dev`; with `env: prod` that is a start-up error, because the embedded key ships in every release.
- **`pkg/relay`** — hardware-facing only: one GPIO output per relay, `TurnOn`/`TurnOff`/`GetState`/`GPIO`. `New` opens the pin via `gpio/rpi` and switches it off; `NewWithPin` takes any `gpio.Pin` (tests use `rpiemu`). It knows nothing of names, descriptions or configuration.

`app.relays` is shared state protected by `app.mu`: reads under `RLock`, writes under `Lock`.

**External dependency `github.com/womat/golib`** supplies `gpio`/`gpio/rpi` (and `gpio/rpiemu` for the tests) and `web` (auth, CORS, IP filter, `Encode`). It is not vendored — read it in `$(go env GOMODCACHE)/github.com/womat/golib@<version>` when behavior is unclear.

## Conventions

- Logging is `log/slog` with key/value pairs throughout; `slog.SetDefault` is set once per lifecycle in `cmd`. Do not use `fmt.Print` outside pre-logger startup and `--about`/`--version`/`--help`.
- Doc comments: every package and exported symbol is documented, in English, and Swagger annotations live directly on the handlers.
- Config field docs live in `README.md` and `config/config.yaml` — update both when adding a config key (the README embeds a copy of the example config). `cmd/README.md` is the short `--help` text.
- Commit subjects follow Conventional Commits, `type(scope): description` with an optional scope (`fix: default port 8443`, `feat(ui): …`), types `feat`, `fix`, `docu`, `chore`, `refactor`. The release changelog groups on them (`.goreleaser.yaml`).
