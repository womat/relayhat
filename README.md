# relayhat

relayhat is a Go service that exposes an HTTPS REST API for BC Robotics Relay HATs on Raspberry Pi.

It supports the 2-channel Pi Zero Relay HAT and the 4-channel Relay HAT, loads its relay layout from YAML config, and
protects requests with an API key.

---

## Features

-  supports the **2-channel Pi Zero Relay HAT** and the **4-channel Relay HAT**
- Exposes a secured **HTTPS REST API** (API key authentication)
- **IP allowlist / blocklist** support
- **Hot-reload** of configuration via `SIGHUP`
- Embedded self-signed TLS certificate for development (no setup required)
- Optional **Swagger UI** (build tag `swagger`, dev only)

---

## Where to start

- Runtime, API, build, deploy, and Swagger usage: [`cmd/README.md`](cmd/README.md)
- Example configuration: [`config/config.yaml`](config/config.yaml)
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
| GET    | `/relays`                | API Key | List all relays          
| GET    | `/relays/{name}`         | API Key | Get relay state          |
| PATCH  | `/relays/{name}/{state}` | API Key | Set relay (`on` / `off`) |

Authentication via the `X-API-Key` header.

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

Default location: `/opt/relayhat/etc/config.yaml`
Environment variables are expanded inside the file, e.g. `apiKey: ${TADL_API_KEY}`.

```yaml
# logLevel defines the minimum log level.
# Messages with at least this level are logged.
# Allowed values: debug | info | warn | error
logLevel: info

# logDestination defines where logs are written to.
# Supported values: stdout | stderr | /path/to/logfile
logDestination: stdout

# environment: dev | prod
env: dev

# =============================================================================
# Webserver configuration (HTTPS)
# =============================================================================
webserver:
  # Host address the HTTPS server listens on (0.0.0.0 = all interfaces)
  listenHost: 0.0.0.0

  # Port the HTTPS server listens on
  listenPort: 8443

  # Global API key for protected endpoints
  apiKey: changeme!

  # TLS private key file
  keyFile: /opt/relayhat/etc/key.pem

  # TLS certificate file
  certFile: /opt/relayhat/etc/cert.pem

  # Blocked IP addresses or networks (empty = none blocked)
  # Examples: 192.168.0.1, 192.168.0.0/16, 10.0.0.0/8
  blockedIPs: [ ]
  #  - 192.168.0.1
  #  - 192.168.0.0/16

  # Allowed IP addresses or networks (empty = all allowed)
  # Note: ::1 is the IPv6 loopback address
  # Examples: 127.0.0.1, ::1, 192.168.0.0/16
  allowedIPs: [ ]
  #  - 127.0.0.1
  #  - ::1
  #  - 192.168.0.0/16

# =============================================================================
# relay configuration
# =============================================================================
relay:
  relay1:
    description: "gpio 4 available for Raspberry Pi 4 Channel Relay HAT and Raspberry Pi Zero Relay HAT"
    gpio: 4
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

For development the application falls back to an embedded self-signed certificate automatically.
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

### 1. Create system user and directories

```sh
sudo groupadd -f relayhat
sudo useradd -r -s /usr/sbin/nologin -g relayhat relayhat
sudo usermod -aG gpio relayhat
sudo mkdir -p /opt/relayhat/{bin,etc,data}
sudo chown -R relayhat:relayhat /opt/relayhat
```

### 2. Copy files

```sh
sudo cp relayhat /opt/relayhat/bin/
sudo cp config.yaml /opt/relayhat/etc/
sudo cp cert.pem key.pem /opt/relayhat/etc/
sudo chown -R relayhat:relayhat /opt/relayhat
```

### 3. Create systemd service

```sh
sudo tee /etc/systemd/system/relayhat.service > /dev/null <<'EOF'
[Unit]
Description=service relayHAT
After=network.target

[Service]
User=relayhat
Group=relayhat
Type=simple
ExecStart=/opt/relayhat/bin/relayhat 
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable relayhat
sudo systemctl start relayhat
sudo systemctl status relayhat
```

### 4. View logs

```sh
journalctl -u relayhat -n 50 -f
```

---

## Build

```sh
# Raspberry Pi 4/5 (64-bit OS)
make build_arm64

# Raspberry Pi 2/3/4 (32-bit OS)
make build_arm7

# Raspberry Pi 1 / Zero (32-bit OS)
make build_arm6

# Build with Swagger UI (dev only)
make build_arm64_dev

# Build and deploy to Raspberry Pi via SCP
make deploy
```

---

## Hot-Reload

Send `SIGHUP` to reload the configuration without restarting the process:

```sh
sudo systemctl reload relayhat
# or
kill -HUP $(pidof relayhat)
```

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

---

# License

MIT