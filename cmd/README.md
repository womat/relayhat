# relayhat

**relayhat** switches the relays of a Raspberry Pi relay HAT through an HTTPS REST API and a built-in web page.

It supports the BC Robotics 2-channel Pi Zero Relay HAT and 4-channel Relay HAT (and any active-high relay board on
GPIO 2–27), loads its relay layout from a YAML config and protects requests with bearer tokens.

---

## Usage

```text
relayhat [--config FILE] [--debug] [--version] [--about] [--help]
relayhat token add <name> [--config FILE] [--dry-run]
relayhat token remove <name> [--config FILE] [--dry-run]
relayhat token list [--config FILE]
```

| Flag        | Default                         | Description                                       |
|-------------|---------------------------------|---------------------------------------------------|
| `--config`  | `/opt/relayhat/etc/config.yaml` | Path to the configuration file                    |
| `--debug`   | `false`                         | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                         | Print the application version and exit            |
| `--about`   | `false`                         | Print application details and exit                |
| `--help`    | `false`                         | Print this help message and exit                  |

The config file path can also be set via the environment variable `CONFIG_FILE`; `--config` wins over it.

---

## API

| Method | Path                     | Auth    | Description              |
|--------|--------------------------|---------|--------------------------|
| GET    | `/`                      | –       | Web page                 |
| GET    | `/version`               | –       | App name and version     |
| GET    | `/health`                | Token   | Runtime health metrics   |
| GET    | `/relays`                | Token   | List all relays          |
| GET    | `/relays/{name}`         | Token   | Get relay state          |
| PATCH  | `/relays/{name}/{state}` | Token   | Set relay (`on` / `off`) |

Authentication with `Authorization: Bearer <token>`, HTTPS is TLS 1.3 only. `lastChange.user` names the token of
the last switch. The web page at `/` asks for a token in the browser; switching there takes two taps.

---

## Signals

| Signal             | Effect                                                                                   |
|--------------------|------------------------------------------------------------------------------------------|
| `SIGHUP`           | Reload the config and the token file; a broken file is refused, relays keep their state  |
| `SIGTERM`/`SIGINT` | Graceful stop, all relays are switched off                                               |

A config that does not load or validate at start ends relayhat with exit code 78.

---

## Tokens

The API except `/` and `/version` needs `Authorization: Bearer <token>`. relayhat keeps only the SHA-256 hash of each
token, in the file named by `webserver.tokenFile`, one `<name> <hash>` per line.

- `relayhat token add <name>` writes the hash and prints the token once, on stdout alone.
- `relayhat token remove <name>` deletes it, `relayhat token list` prints the names.
- `--dry-run` prints what would be done and writes nothing.
- `systemctl reload relayhat` applies a change; the relays keep their state.

Run them with `sudo`: the token file belongs to root.

---

Configuration, installation, TLS and build: see `README.md` in the repository,
https://github.com/womat/relayhat
