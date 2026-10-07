# relayhat

**relayhat** is a Go service that exposes an HTTPS REST API for BC Robotics Relay HATs on Raspberry Pi.

It supports the 2-channel Pi Zero Relay HAT and the 4-channel Relay HAT, loads its relay layout from YAML config, and
protects requests with an API key.

---

## Usage

```text
relayhat [--config FILE] [--debug] [--version] [--about] [--help]
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
| GET    | `/version`               | –       | App name and version     |
| GET    | `/health`                | API Key | Runtime health metrics   |
| GET    | `/relays`                | API Key | List all relays          |
| GET    | `/relays/{name}`         | API Key | Get relay state          |
| PATCH  | `/relays/{name}/{state}` | API Key | Set relay (`on` / `off`) |

Authentication via the `X-API-Key` header.

---

## Signals

| Signal             | Effect                                                                                   |
|--------------------|------------------------------------------------------------------------------------------|
| `SIGHUP`           | Reload the config; a broken file is refused, configured relays keep their state           |
| `SIGTERM`/`SIGINT` | Graceful stop, all relays are switched off                                               |

---

Configuration, installation, TLS and build: see `README.md` in the repository,
https://github.com/womat/relayhat
