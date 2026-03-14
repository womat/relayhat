# relayhat

HTTP/S REST API to control BC Robotics Relay HATs on Raspberry Pi.

Supports the **2-Channel Relay HAT** (Pi Zero) and the **4-Channel Relay HAT** (Pi 3/4/5).

## Features

- REST API with API key and JWT authentication
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

## API Endpoints

| Method | Path                   | Auth | Description              |
|--------|------------------------|------|--------------------------|
| GET    | /version               | –    | App name and version     |
| GET    | /health                | ✓    | Runtime health metrics   |
| GET    | /relays                | ✓    | List all relays          |
| GET    | /relays/{name}         | ✓    | Get relay state          |
| PUT    | /relays/{name}/{state} | ✓    | Set relay (`on` / `off`) |

Authentication via `X-API-Key` header or JWT Bearer token.

### Examples
 
```bash
# Get all relays
curl -k https://raspberrypi/relays \
  -H "X-API-Key: your-secret-key"
 
# Get a single relay
curl -k https://raspberrypi/relays/relay1 \
  -H "X-API-Key: your-secret-key"
 
# Turn relay on
curl -k -X PUT https://raspberrypi/relays/relay1/on \
  -H "X-API-Key: your-secret-key"
 
# Turn relay off
curl -k -X PUT https://raspberrypi/relays/relay1/off \
  -H "X-API-Key: your-secret-key"
 
# Using JWT Bearer token instead of API key
curl -k https://raspberrypi/relays \
  -H "Authorization: Bearer <token>"
```

## Configuration

Copy `config/config.yaml` to `/opt/relayhat/etc/config.yaml` and adjust:

```yaml
webserver:
  listenPort: 443
  apiKey: your-secret-key
  certFile: /opt/relayhat/etc/cert.pem
  keyFile: /opt/relayhat/etc/key.pem

relay:
  relay1:
    gpio: 4
    description: "Channel 1"
  relay2:
    gpio: 17
    description: "Channel 2"
```

## Build

```bash
# Raspberry Pi 3/4/5/Zero2 – 64-bit
make build_arm64
 
# Raspberry Pi Zero / 1 – 32-bit ARMv6
make build_arm6
 
# Raspberry Pi 2 / 3 / 4 – 32-bit ARMv7
make build_arm7
```

## Deploy

```bash
make deploy   # builds arm64 and copies binary to Pi via scp
```

Then on the Pi:

```bash
sudo systemctl stop relayhat
sudo cp ~/relayhat /opt/relayhat/bin/relayhat
sudo systemctl start relayhat
```

## Generate Swagger Docs

```bash
go install github.com/swaggo/swag/cmd/swag@latest
docs/generate.sh
```

## Systemd

```ini
[Unit]
Description=relayhat
After=network.target

[Service]
ExecStart=/opt/relayhat/bin/relayhat --config /opt/relayhat/etc/config.yaml
Restart=on-failure
User=pi

[Install]
WantedBy=multi-user.target
```

## License

MIT