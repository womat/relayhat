# relayhat

# relayhat

relayhat is a Go service that exposes an HTTPS REST API for BC Robotics Relay HATs on Raspberry Pi.

It supports the 2-channel Pi Zero Relay HAT and the 4-channel Relay HAT, loads its relay layout from YAML config, and
protects requests with an API key.

## Features

- REST API with API key authentication
- HTTPS with auto-fallback to embedded self-signed cert (dev)
- IP allowlist / blocklist
- Graceful shutdown and hot-reload via SIGHUP
- Swagger UI (optional build tag)
- GPIO relay control via [go-gpiocdev](https://github.com/warthog618/go-gpiocdev)

## Hardware

| Board                         | GPIO Pins used     |
|-------------------------------|--------------------|
| Pi Zero Relay HAT (2-channel) | GPIO 4, 17         |
| Pi 4-Channel Relay HAT        | GPIO 4, 17, 22, 27 |

---

## Command-Line Flags

| Flag        | Default                     | Description                                       |
|-------------|-----------------------------|---------------------------------------------------|
| `--config`  | `/opt/tadl/etc/config.yaml` | Path to the configuration file                    |
| `--debug`   | `false`                     | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                     | Print the application version and exit            |
| `--about`   | `false`                     | Print application details and exit                |
| `--help`    | `false`                     | Print this help message and exit                  |

The config file path can also be set via the environment variable `CONFIG_FILE`.

```sh
tadl --config /etc/tadl/config.yaml
tadl --debug
tadl --version
CONFIG_FILE=/etc/tadl/config.yaml tadl
```

---

## Configuration

The configuration file is a YAML file. By default it is loaded from `/opt/relayhat/etc/config.yaml`.
Environment variables are expanded inside the file, e.g. `apiKey: ${TADL_API_KEY}`.
