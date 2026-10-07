# relayhat

relayhat is a Go service that exposes an HTTPS REST API for BC Robotics Relay HATs on Raspberry Pi.

It supports the 2-channel Pi Zero Relay HAT and the 4-channel Relay HAT, loads its relay layout from YAML config, and
protects requests with an API key.

---

## Features

- Supports the **2-channel Pi Zero Relay HAT** and the **4-channel Relay HAT**
- Exposes a secured **HTTPS REST API** (API key authentication)
- **IP allowlist / blocklist** support
- **Hot-reload** of configuration via `SIGHUP`; relays keep their state, a broken config is refused
- Embedded self-signed TLS certificate for development (`env: dev` only)
- Optional **Swagger UI** (build tag `swagger`, dev only)

---

## Where to start

- This README is the reference for API, configuration and installation.
- [`cmd/README.md`](cmd/README.md) is the short `--help` text of the binary.
- Example configuration: [`config/config.yaml`](config/config.yaml)
- Building, testing and releasing: [`CLAUDE.md`](CLAUDE.md)
- Swagger generation script: [`docs/generate.sh`](docs/generate.sh)

---

## Supported hardware

| Board                         | GPIO pins     |
|-------------------------------|---------------|
| Pi Zero Relay HAT (2-channel) | 4, 17         |
| Pi 4-Channel Relay HAT        | 4, 17, 22, 27 |

---

## API Endpoints

| Method | Path                     | Auth    | Description              |
|--------|--------------------------|---------|--------------------------|
| GET    | `/version`               | –       | App name and version     |
| GET    | `/health`                | API Key | Runtime health metrics   |
| GET    | `/relays`                | API Key | List all relays          |
| GET    | `/relays/{name}`         | API Key | Get relay state          |
| PATCH  | `/relays/{name}/{state}` | API Key | Set relay (`on` / `off`) |

Authentication via the `X-API-Key` header. Errors are returned as `{"error": "..."}` with the HTTP status
(401, 404, 400 for an invalid state); a 500 carries only `internal server error`, the cause is in the log.
Every switch is logged with relay, GPIO, old and new state and the client address.

### Examples

```bash
# Get all relays
curl -k https://localhost:8443/relays \
  -H "X-API-Key: your-secret-key"
 
# Get a single relay
curl -k https://localhost:8443/relays/relay1 \
  -H "X-API-Key: your-secret-key"
 
# Turn relay on
curl -k -X PATCH https://localhost:8443/relays/relay1/on \
  -H "X-API-Key: your-secret-key"
 
# Turn relay off
curl -k -X PATCH https://localhost:8443/relays/relay1/off \
  -H "X-API-Key: your-secret-key"
 
```

---

## Command-Line Flags

| Flag        | Default                     | Description                                       |
|-------------|-----------------------------|---------------------------------------------------|
| `--config`  | `/opt/relayhat/etc/config.yaml` | Path to the configuration file                    |
| `--debug`   | `false`                     | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                     | Print the application version and exit            |
| `--about`   | `false`                     | Print application details and exit                |
| `--help`    | `false`                     | Print this help message and exit                  |

The config file path can also be set via the environment variable `CONFIG_FILE`; `--config` wins over it.

```sh
relayhat --config /etc/relayhat/config.yaml
relayhat --debug
relayhat --version
CONFIG_FILE=/etc/relayhat/config.yaml relayhat
```

---

## Configuration

Default location: `/opt/relayhat/etc/config.yaml`

- `${VAR}` is replaced with the environment variable `VAR`, e.g. `apiKey: ${RELAYHAT_API_KEY}`. Only this form is
  expanded; a bare `$` stays as it is, so keys containing `$` are safe.
- Unknown keys are an error, so a typo cannot silently fall back to a default.
- The configuration is validated on start and before every reload: `env` is `dev` or `prod`, `apiKey` is set,
  every relay uses a GPIO between 2 and 27, and no GPIO is used twice.
- A weak `apiKey` (the example value or shorter than 16 characters) does not stop the service but is logged as a
  warning.
- `startState` per relay sets the state after the process starts: `off` (default), `on`, or `last` – the state
  before, read from `stateFile`. A missing, empty or damaged state file is not an error; the relays with `last` then
  start off and the log says why.

```yaml
# =============================================================================
# relayhat configuration
#
# ${VAR} is replaced with the environment variable VAR (only this form, a bare
# "$" stays as it is), e.g. apiKey: ${RELAYHAT_API_KEY}.
# Unknown keys are an error, so a typo cannot silently fall back to a default.
# =============================================================================

# logLevel defines the minimum log level.
# Messages with at least this level are logged.
# Allowed values: debug | info | warn | error
logLevel: info

# logDestination defines where logs are written to.
# Supported values: stdout | stderr | null | /path/to/logfile
logDestination: stdout

# environment: dev | prod
# dev falls back to the embedded self-signed certificate when certFile is
# missing; prod refuses to start without certFile.
env: dev

# stateFile keeps the last state of every relay for startState: last. It is
# written after every switch and once on start; the directory must exist and be
# writable. Only used when a relay has startState: last.
stateFile: /opt/relayhat/data/state.yaml

# =============================================================================
# Webserver configuration (HTTPS)
# =============================================================================
webserver:
  # Host address the HTTPS server listens on (0.0.0.0 = all interfaces)
  listenHost: 0.0.0.0

  # Port the HTTPS server listens on
  listenPort: 8443

  # Global API key for protected endpoints, sent as X-API-Key header.
  # Use a random key of at least 16 characters; the example value is logged as a warning.
  apiKey: changeme!

  # TLS private key file
  keyFile: /opt/relayhat/etc/key.pem

  # TLS certificate file
  certFile: /opt/relayhat/etc/cert.pem

  # Blocked IP addresses or networks (empty = none blocked)
  # Examples: 192.168.0.1, 192.168.0.0/16, 10.0.0.0/8
  blockedIPs: []
  #  - 192.168.0.1
  #  - 192.168.0.0/16

  # Allowed IP addresses or networks (empty = all allowed)
  # Note: ::1 is the IPv6 loopback address
  # Examples: 127.0.0.1, ::1, 192.168.0.0/16
  allowedIPs: []
  #  - 127.0.0.1
  #  - ::1
  #  - 192.168.0.0/16

# =============================================================================
# relay configuration
#
# name: relay name used in the API (/relays/{name})
# gpio: BCM number 2..27, each gpio at most once
# startState: state after the process starts (boot, crash, systemctl restart):
#   off  (default) switched off
#   on   switched on
#   last the state before, from stateFile; off if it has none (first start,
#        missing or damaged file)
# A SIGHUP reload keeps the state of every relay whose gpio stays configured;
# startState applies only to relays that are added by the reload.
# A stop (SIGTERM/SIGINT) switches all relays off.
# =============================================================================
relay:
  relay1:
    description: "gpio 4 available for Raspberry Pi 4 Channel Relay HAT and Raspberry Pi Zero Relay HAT"
    gpio: 4
    startState: off
  relay2:
    description: "gpio 17 available for Raspberry Pi 4 Channel Relay HAT and Raspberry Pi Zero Relay HAT"
    gpio: 17
  relay3:
    description: "gpio 22 only for Raspberry Pi 4 Channel Relay HAT"
    gpio: 22
  relay4:
    description: "gpio 27 only for Raspberry Pi 4 Channel Relay HAT"
    gpio: 27
```

---

## TLS Certificate

With `env: dev` the application falls back to an embedded self-signed certificate when `certFile` does not exist.
With `env: prod` a missing `certFile` is a start-up error, because the embedded private key ships in every binary.
For production, generate your own:

```sh
openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout /opt/relayhat/etc/key.pem \
  -out    /opt/relayhat/etc/cert.pem \
  -days 825 \
  -subj "/C=AT/ST=Vienna/L=Vienna/O=MyOrg/CN=localhost"
```

**Subject fields:**

| Field           | Example             | Description                                  |
|-----------------|---------------------|----------------------------------------------|
| `/C`            | `AT`                | Country code (2 letters)                     |
| `/ST`           | `Vienna`            | State or province (optional)                 |
| `/L`            | `Vienna`            | City (optional)                              |
| `/O`            | `MyCompany`         | Organization (optional)                      |
| `/OU`           | `DEV`               | Organizational unit (optional)               |
| `/CN`           | `localhost`         | **Common Name — your domain or `localhost`** |
| `/emailAddress` | `admin@example.com` | E-mail address (optional)                    |

> **Note:** Browsers enforce a maximum certificate validity of 825 days. Use `-days 365` for production-like setups.


---

## Installation

**1. Download** the archive for your Pi from the [latest release](https://github.com/womat/relayhat/releases/latest):

| Archive        | Raspberry Pi model                               |
|----------------|--------------------------------------------------|
| `linux_armv6`  | Pi 1 and Zero (1st gen), runs on every Pi        |
| `linux_armv7`  | Pi 2 / 3 / 4 / 5 / Zero 2 W with a 32-bit OS     |
| `linux_arm64`  | Pi 3 / 4 / 5 / 400 / Zero 2 W with a 64-bit OS   |

```sh
VERSION=1.7.0 ARCH=armv6        # see the release page for the latest version
BASE=https://github.com/womat/relayhat/releases/download/v$VERSION
curl -LO $BASE/relayhat_${VERSION}_linux_$ARCH.tar.gz -LO $BASE/checksums.txt
sha256sum -c checksums.txt --ignore-missing
tar xzf relayhat_${VERSION}_linux_$ARCH.tar.gz
```

**2. Install** binary, example configuration and a certificate:

```sh
sudo groupadd -r -f relayhat
sudo useradd -r -s /usr/sbin/nologin -g relayhat relayhat
sudo usermod -aG gpio relayhat
sudo mkdir -p /opt/relayhat/{bin,etc,data}

sudo install -m 755 relayhat /opt/relayhat/bin/
sudo install -m 640 config/config.yaml /opt/relayhat/etc/
sudo openssl req -x509 -nodes -newkey rsa:2048 -days 825 \
  -keyout /opt/relayhat/etc/key.pem -out /opt/relayhat/etc/cert.pem -subj "/CN=$(hostname)"
sudo chown -R relayhat:relayhat /opt/relayhat
```

**3. Configure** `/opt/relayhat/etc/config.yaml`: set `env: prod`, a random `apiKey`
(`openssl rand -hex 24`) and one entry per relay — see [Configuration](#configuration).

**4. Start** it as a service:

```sh
sudo tee /etc/systemd/system/relayhat.service > /dev/null <<'EOF'
[Unit]
Description=relayhat - Relay HAT REST API
After=network-online.target
Wants=network-online.target

[Service]
User=relayhat
Group=relayhat
Type=simple
ExecStart=/opt/relayhat/bin/relayhat
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now relayhat
journalctl -u relayhat -n 20      # "Module started successfully"
```

---

## Releases

Every release on the [releases page](https://github.com/womat/relayhat/releases) carries archives for
all Raspberry Pi architectures with the binary, `config/config.yaml`, `README.md` and `LICENSE`,
plus a `checksums.txt` and a changelog. Versions follow [semantic versioning](https://semver.org/);
a breaking change of the API or the configuration raises the major version.

`relayhat --version` reports the release a binary was built from. A local build reports something
like `1.7.0-3-g0c13781-dirty` instead, which is how the two are told apart on a device.

Building from source needs Go and `make`: clone the repository and run `make help` for the targets.

---

## Hot-Reload

Send `SIGHUP` to reload the configuration without restarting the process:

```sh
sudo systemctl reload relayhat
# or
kill -HUP $(pidof relayhat)
```

The config file is validated first; if it is broken, the reload is refused, logged, and the service keeps running
unchanged. Relays whose GPIO is still configured keep their state across the reload (also when they are renamed),
removed relays are switched off, new ones start in their `startState`.

| Event                                       | Relay state afterwards                          |
|---------------------------------------------|-------------------------------------------------|
| `SIGHUP` reload                             | unchanged                                       |
| process start (boot, crash, `systemctl restart`) | `startState`: `off`, `on` or `last`        |
| stop (`SIGTERM`/`SIGINT`, `systemctl stop`) | off                                             |

---

## Firewall

```sh
# Allow the configured port (default 8443)
sudo ufw allow 8443/tcp
sudo ufw status
```

---

## Backup & Restore

```sh
# Backup
sudo tar czf /tmp/relayhat-backup.tar.gz /opt/relayhat

# Restore
sudo tar xzf /tmp/relayhat-backup.tar.gz -C /
sudo chown -R relayhat:relayhat /opt/relayhat
sudo systemctl restart relayhat
```

---

## License

relayhat is released under the MIT License - see [`LICENSE`](LICENSE) for the full text.

### Third-party licenses

The source tree contains no third-party code, but a **compiled binary statically links** the
modules below. Their terms apply to anyone distributing that binary, not to the sources here.

| Module                                            | License                |
|---------------------------------------------------|------------------------|
| `github.com/womat/golib`                          | MIT                    |
| `github.com/warthog618/go-gpiocdev`               | MIT                    |
| `github.com/golang-jwt/jwt/v5`                    | MIT                    |
| `gopkg.in/yaml.v3`                                | MIT and Apache-2.0     |
| `golang.org/x/sys`                                | BSD-3-Clause           |
| Swagger UI build only (`-tags swagger`):          |                        |
| `github.com/swaggo/swag`, `http-swagger`, `files` | MIT                    |
| `github.com/go-openapi/*`, `go.yaml.in/yaml/v3`   | Apache-2.0             |
| `github.com/KyleBanks/depth`                      | MIT                    |
| `golang.org/x/net`, `mod`, `sync`, `tools`        | BSD-3-Clause           |

All of these are permissive; none obliges relayhat to change its license.

---

# Hardware reference

The following hardware notes are kept here intentionally as reference material for the supported BC Robotics boards.

---

## Description Raspberry Pi Zero Relay HAT

With the addition of WiFi and Bluetooth to the Raspberry Pi Zero W, it is now finding itself in many more IoT
applications. This 2 Channel Relay HAT makes driving higher current and higher voltage devices as easy as possible! This
premium Relay HAT matches the Raspberry Pi Zero form factor and is compatible with all versions of the Pi Zero. Each 10A
relay has the Input, Normally Open, and Normally Closed contact broken out to a nice 5mm pitch screw terminal. The board
uses high quality North American sourced parts, a locally produced circuit board, and simple logic level inputs.

This HAT is compatible with the Raspberry Pi Zero / Zero 1.3 / Zero W and uses GPIO pins 4 and 17 (Pins 7 & 11) on the
GPIO header. Each relay driver is connected to the GPIO through a solder jumper and can be alternatively connected
through the 2 pin 0.100″ header if a custom configuration is required.

This board does not ship with a header – we recommend the GPIO Header for Raspberry Pi HAT. Standoffs, like those
included in our HAT hardware kit , are ideal for ensuring everything stays in place!

Please Note: While this board is capable of switching higher voltages, please exercise caution. If you are inexperienced
or unsure about how to use this product safely we recommend looking at the IoT Power Relay, which has all of its high
voltage circuitry fully enclosed.

### Features

Fits directly on the Pi's 40 pin GPIO header
Uses GPIO 4 and GPIO 17
Switch up to 10A per channel!
Compatible with all models of the Raspberry Pi Zero

---

## Description Raspberry Pi 4 Channel Relay HAT

Need to drive high current or high voltage devices with your Raspberry Pi? This premium 4 channel 10A relay HAT can
handle it! Each relay has the Input, Normally Open, and Normally Closed contact broken out to a nice 5mm pitch screw
terminal. This HAT is compatible with the Raspberry Pi A+/B+/2/3B/3+/4 and uses GPIO pins 4, 17, 27, and 22 (Pins
7,11,13,15) on the GPIO header. Each relay driver is connected to the GPIO through a solder jumper and can be
alternatively connected through the 4 pin 0.100″ header if a custom configuration is required.

This board does not ship with a header so minor soldering will be required before it can be used with the Pi. We
recommend a Tall GPIO Header for this board.

As of September 14th 2017 we are now shipping version 1.1 of this board. Version 1.1 adds isolation routing to the board
and a few SMD components have been shifted around. It is otherwise functionally identical. This board Works with all “A”
and “B” versions of the Raspberry Pi (Pi A+, B+, 2, 3, 3A+, 3B+, and Pi 4)

Please Note: While this board is capable of switching higher voltages, please exercise caution. If you are inexperienced
or unsure about how to use this product safely we recommend looking at the IoT Power Relay, which has all of its high
voltage circuitry fully enclosed.

### Features

Fits directly on the Pi's 40 pin GPIO header
Uses GPIO 4,17,27,22
Switch up to 10A per channel!
Compatible with all models of the Raspberry Pi Zero

---

## Getting Started With The Raspberry Pi Relay HAT

Our Pi Relay HATs are designed to allow your Pi to switch higher voltages and higher currents from one self contained
board. In this tutorial we are going to go over soldering the header to the Relay HAT, use Python with the included
Pi.GPIO library to write code that triggers each relay, and go over the external relay connections and configuration
options on the board

About The Boards:
We make two versions of this relay board, one for the standard Raspberry Pi with 4 relays, and one for the Raspberry Pi
Zero with 2 relays. On the board, each relay’s Common, Normally Open, and Normally Closed pins are brought out to screw
terminals. These are not the most sophisticated circuits, but they do provide a compact, permanent solution for
attaching a number of relays to the Pi.

---

## A Quick Overview

There isn’t much in the way of assembly required with these boards as they ship with everything but a header installed.
We do not solder a header to the board as different heights or types may be required depending on your application (and
removing them can be quite a pain!).

For all standard Raspberry Pi we recommend using a Tall Header, as this will allow inputs on the “USB / Ethernet” side
of the Pi to clear. For the Pi Zero, a shorter header can be used so we recommend using the standard GPIO header, but
feel free to go a different way as needed. Once we have the headers soldered in, we will use Python with the included
Pi.GPIO library to write code that triggers each relay, and finally we will look at the different connections on the
board.

---

## Links

- https://bc-robotics.com/shop/raspberry-pi-zero-relay-hat/
- https://bc-robotics.com/shop/raspberry-pi-4-channel-relay-hat/
- https://bc-robotics.com/tutorials/getting-started-raspberry-pi-relay-hat/
