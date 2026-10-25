# relayhat

**Switch the relays of a Raspberry Pi relay HAT from your browser, Node-RED or Home Assistant.**

[![CI](https://github.com/womat/relayhat/actions/workflows/ci.yml/badge.svg)](https://github.com/womat/relayhat/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/womat/relayhat)](https://github.com/womat/relayhat/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/womat/relayhat)](go.mod)
![Raspberry Pi](https://img.shields.io/badge/runs%20on-Raspberry%20Pi-C51A4A)

🇩🇪 [Deutsche Kurzfassung](README.de.md)

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/web-ui-dark.png">
    <img src="docs/screenshots/web-ui.png" width="640" alt="relayhat web page: two relays as cards with a pilot light, the utility lock waiting for the confirming second tap">
  </picture>
  &nbsp;
  <img src="docs/screenshots/web-ui-phone.png" width="180" alt="The same page on a phone">
</p>

> **Got a Pi with a relay HAT?** The [Quick start](#quick-start) gets you from download to switching
> in the browser in about ten minutes.

A relay HAT turns a Raspberry Pi into a switch for things the Pi cannot drive itself: a heat pump's
utility lock or PV surplus input, a pump, a gate, a light. relayhat makes those relays usable from
everywhere on your network:

- a **REST API** over HTTPS with one bearer token per caller, for Node-RED, Home Assistant, ioBroker, openHAB or a
  `curl` in a script,
- a **built-in web page** with a pilot light per relay and a **two-step switch** that a stray tap
  cannot trigger,
- relays that **keep their state** when the configuration is reloaded and come back in a state you
  choose (`off`, `on` or the **last** one) after a reboot or power cut,
- a **switch lock** per relay against rapid toggling, and a record of **who switched last**.

No cloud, no database, no runtime: a single binary, configured with one YAML file.

## Features

- **Web page** per device: pilot light in the relay's colour, your own words for on and off
  ("Locked" / "Released"), time in the current state, last switch with the name of the caller's token —
  readable on a phone, light and dark mode
- **Two-step switching** in the browser: the first tap arms the button for 3 seconds, the second
  switches; a double tap, `Esc` or the timeout cancel
- **Start state** per relay: `off`, `on` or `last`, restored from a state file that is written
  atomically, so a power cut cannot corrupt it
- **Switch lock** (`minSwitchInterval`): a relay refuses to switch again within, say, 5 seconds or
  10 minutes of the last switch — for every client, answered with HTTP 429
- **Audit trail**: every switch is logged with relay, old and new state, token name and client; the
  last one is shown on the page and kept across restarts
- **HTTPS REST API** (TLS 1.3) with one bearer token per caller, stored only as a SHA-256 hash and
  managed with `relayhat token`; IP allowlist / blocklist, JSON errors
- **Hot reload** via `SIGHUP`: a broken file is refused, the relays do not even flicker
- Release builds for every Raspberry Pi architecture, from the **Pi Zero (ARMv6)** to 64-bit systems

---

## Quick start

**1. Download** the archive for your Pi from the [latest release](https://github.com/womat/relayhat/releases/latest):

| Archive        | Raspberry Pi model                                    |
|----------------|-------------------------------------------------------|
| `linux_armv6`  | Pi 1 and Zero (1st gen); also runs on every newer Pi |
| `linux_armv7`  | Pi 2 / 3 / 4 / 5 / Zero 2 W with a 32-bit OS          |
| `linux_arm64`  | Pi 3 / 4 / 5 / 400 / Zero 2 W with a 64-bit OS        |

```sh
VERSION=2.0.1 ARCH=armv6        # see the release page for the latest version
BASE=https://github.com/womat/relayhat/releases/download/v$VERSION
curl -LO $BASE/relayhat_${VERSION}_linux_$ARCH.tar.gz -LO $BASE/checksums.txt
sha256sum -c checksums.txt --ignore-missing
tar xzf relayhat_${VERSION}_linux_$ARCH.tar.gz
```

**2. Install** binary, example configuration and a certificate. Everything belongs to root except
`data/`, where relayhat keeps the relay states; the service user `relayhat` may read `etc/`:

```sh
sudo groupadd -r -f relayhat
sudo useradd -r -s /usr/sbin/nologin -g relayhat relayhat
sudo usermod -aG gpio relayhat
sudo install -d -o root -g root -m 755 /opt/relayhat /opt/relayhat/bin
sudo install -d -o root -g relayhat -m 750 /opt/relayhat/etc
sudo install -d -o relayhat -g relayhat -m 750 /opt/relayhat/data

sudo install -o root -g root -m 755 relayhat /opt/relayhat/bin/
sudo install -o root -g relayhat -m 640 config/config.yaml /opt/relayhat/etc/
sudo openssl req -x509 -nodes -newkey rsa:2048 -days 825 \
  -keyout /opt/relayhat/etc/key.pem -out /opt/relayhat/etc/cert.pem -subj "/CN=$(hostname)"
sudo chown root:relayhat /opt/relayhat/etc/key.pem /opt/relayhat/etc/cert.pem
sudo chmod 640 /opt/relayhat/etc/key.pem
```

Why root: whoever can write `config.yaml` or the token file can add a token of their own, and
whoever can write `bin/relayhat` can replace the program — and both switch your relays.

**3. Configure** `/opt/relayhat/etc/config.yaml`: set `env: prod` and one entry per relay — see
[Configuration](#configuration) and [Hardware](#hardware). Then create a token for each caller,
e.g. the browser and Node-RED:

```sh
sudo /opt/relayhat/bin/relayhat token add browser
sudo /opt/relayhat/bin/relayhat token add nodered
```

Each prints its token once; keep it, e.g. in a password manager. See [Tokens](#tokens).

**4. Start** it as a service, with the unit from the archive, and open the firewall:

```sh
sudo install -m 644 contrib/relayhat.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now relayhat
sudo ufw allow 8443/tcp          # if ufw is active
journalctl -u relayhat -n 20     # "Module started successfully"
```

The unit runs relayhat in a sandbox: no capabilities, a read-only system except `data/`, only the
GPIO chip as a device. A `logDestination` file has to go to `data/` as well.

**5. Open** `https://<your-pi>:8443/`, accept the self-signed certificate and enter the token.

---

## Hardware

relayhat was written for the relay HATs of [BC Robotics](https://bc-robotics.com/) and runs on
every Raspberry Pi with a 40-pin header. It drives one GPIO output per relay: **high switches the
relay on**.

| Board | Relays | GPIO (BCM) | Header pins |
|---|---|---|---|
| [Raspberry Pi Zero Relay HAT](https://bc-robotics.com/shop/raspberry-pi-zero-relay-hat/) | 2 | 4, 17 | 7, 11 |
| [Raspberry Pi 4 Channel Relay HAT](https://bc-robotics.com/shop/raspberry-pi-4-channel-relay-hat/) | 4 | 4, 17, 27, 22 | 7, 11, 13, 15 |

Each relay brings its common (COM), normally open (NO) and normally closed (NC) contact out to a
screw terminal. Wire your load to **COM and NO** if it should run while the relay is on, to
**COM and NC** if it should run while it is off — which is also what happens while relayhat is
stopped or the Pi is without power.

Any other relay board with logic-level, **active-high** inputs works the same way: give each relay
its GPIO in the configuration. Boards whose relays switch on when the input is pulled low would
work inverted and are not supported.

> [!WARNING]
> The relays can switch mains voltage. Wiring 230 V / 120 V is a job for a qualified electrician;
> keep it in a closed enclosure, away from the Pi's low-voltage side. If in doubt, switch a
> contactor or an input of the device (such as a heat pump's control input) instead of the load
> itself.

BC Robotics also has a [getting-started tutorial](https://bc-robotics.com/tutorials/getting-started-raspberry-pi-relay-hat/)
for soldering the header and the terminal connections.

---

## Web UI

Open `https://<pi>:<listenPort>/` in a browser. The page asks for a token once
(`relayhat token add browser`), keeps it in the browser's local storage and sends it as a bearer
token; **Sign out** forgets it, and a rejected token brings the prompt back.

- One card per relay with its `label`, `description` and GPIO, a pilot light in the relay's `color`,
  the state (`onText` / `offText`) and how long it has been in it, and the last switch with the
  name of the caller's token (address in the tooltip).
- **Two-step switching**: the first tap arms the button (`Confirm: …`, a bar runs down for 3 seconds),
  only a second tap within that time switches. A fast double tap, `Esc` or the timeout cancel. When
  another client switches the relay meanwhile, the pending confirmation is dropped.
- **Switch lock**: a relay with `minSwitchInterval` shows it below its button; after every switch,
  from the page or any other client, the button is disabled and counts down (`🔒 Locked · 0:04`).
  The lock applies to the page too, there is no override.
- Refreshes every 3 seconds while the tab is visible; if the Pi does not answer, a banner says so
  and the values are greyed out.
- Self-contained: no external fonts or scripts, so it works on a network without internet access.

The page itself is public, as it holds no data; everything it shows and switches needs a
token. Limit who can reach it with `allowedIPs`, e.g. to your home network.

---

## Node-RED and Home Assistant

Switching is a `PATCH` to `/relays/{name}/on` or `/off` with a bearer token; the answer is the
relay's new state. Give every caller a token of its own, so `lastChange.user` says who switched.

**Node-RED**: an *http request* node with method `PATCH`, URL
`https://<your-pi>:8443/relays/relay1/{{payload}}` (send `on` or `off` as `msg.payload`), authentication
*bearer authentication* with its token (`relayhat token add nodered`; Node-RED keeps it in its
encrypted credentials, not in the flow) and a TLS configuration without *Verify server certificate* for the self-signed
certificate.

**Home Assistant**: a `rest_command` per state, used in automations or scripts:

```yaml
rest_command:
  relay1_on:
    url: https://<your-pi>:8443/relays/relay1/on
    method: patch
    headers:
      Authorization: !secret relayhat_bearer   # "Bearer <token>"
    verify_ssl: false
  relay1_off:
    url: https://<your-pi>:8443/relays/relay1/off
    method: patch
    headers:
      Authorization: !secret relayhat_bearer   # "Bearer <token>"
    verify_ssl: false
```

Repeating the current state is harmless: it is answered with 200, does not count as a switch and is
allowed during the switch lock — a flow may send its state on every run.

---

## REST API

| Method | Path                     | Auth    | Description              |
|--------|--------------------------|---------|--------------------------|
| GET    | `/`                      | –       | Web page                 |
| GET    | `/version`               | –       | App name and version     |
| GET    | `/health`                | Token   | Runtime health metrics   |
| GET    | `/relays`                | Token   | List all relays          |
| GET    | `/relays/{name}`         | Token   | Get relay state          |
| PATCH  | `/relays/{name}/{state}` | Token   | Set relay (`on` / `off`) |

Authentication with a bearer token from `relayhat token add`: `Authorization: Bearer <token>`. HTTPS
is TLS 1.3 only. Errors are returned as `{"error": "..."}` with the HTTP
status (401 without a valid token, 404, 400 for an invalid state, 429 with a `Retry-After` header while the relay's switch
lock runs); a 500 carries only `internal server error`, the cause is in the log. Every switch is
logged with relay, GPIO, old and new state, the token name and the client address.

A relay is returned as:

```json
{
  "name": "relay1",
  "state": "off",
  "description": "Utility lock signal of the heat pump",
  "gpio": 4,
  "display": { "label": "Heat pump", "color": "red", "onText": "Locked", "offText": "Released" },
  "lastChange": {
    "time": "2026-10-07T14:02:13+02:00",
    "ageSeconds": 2820.4,
    "source": "api",
    "client": "192.168.1.20",
    "user": "nodered"
  },
  "lock": { "intervalSeconds": 5, "remainingSeconds": 0 }
}
```

`display` comes from the configuration and is meant for the web page. `lastChange` is the last
switch: `source` is `api`, or `start` when relayhat switched the relay to its start state; `client` is
the address and `user` the name of the token of the caller.
`lastChange` is kept in the `stateFile` and survives a restart. `lock` is present with
`minSwitchInterval` only; `remainingSeconds` is how long the relay still refuses to switch. A request
for the state the relay is already in succeeds without switching, so it neither changes `lastChange`
nor starts the lock.

```bash
# Get all relays
curl -k https://<your-pi>:8443/relays -H "Authorization: Bearer $TOKEN"

# Get a single relay
curl -k https://<your-pi>:8443/relays/relay1 -H "Authorization: Bearer $TOKEN"

# Turn a relay on / off
curl -k -X PATCH https://<your-pi>:8443/relays/relay1/on -H "Authorization: Bearer $TOKEN"
curl -k -X PATCH https://<your-pi>:8443/relays/relay1/off -H "Authorization: Bearer $TOKEN"
```

---

## Configuration

Default location: `/opt/relayhat/etc/config.yaml`

- `${VAR}` is replaced with the environment variable `VAR`. Only this form is expanded; a bare `$`
  stays as it is.
- Unknown keys are an error, so a typo cannot silently fall back to a default.
- Durations need a unit (`5s`, `10m`, `0s`); a bare number, `0` included, is rejected.
- The configuration is validated on start and before every reload: `env` is `dev` or `prod`,
  `tokenFile` is set and readable, `certFile` and `keyFile` exist with `env: prod`, every relay uses a GPIO between 2 and 27,
  no GPIO is used twice, and no relay name contains `/` or is `.` or `..` (the name is a URL path
  segment).
- `startState` per relay sets the state after the process starts: `off` (default), `on`, or `last` –
  the state before, read from `stateFile`. A missing, empty or damaged state file is not an error;
  the relays with `last` then start off and the log says why.
- `stateFile` also keeps the last switch of every relay, so the web page shows it after a restart.
  It is written on start and after every switch; `stateFile: ""` keeps nothing. The format of 1.7
  (`relay1: on`) is still read.
- `minSwitchInterval` per relay (e.g. `5s`, `10m`, at most `24h`) refuses a switch within that time
  of the last one with HTTP 429, for every client. It runs from the last switch, also across a reload
  or restart; start states are never refused.
- `label`, `color`, `onText` and `offText` only change how the web page shows a relay; `color` is
  `green`, `red` or `amber`. The relay name stays the API path, so renaming a relay on the page never
  breaks a client.

```yaml
# =============================================================================
# relayhat configuration
#
# ${VAR} is replaced with the environment variable VAR (only this form, a bare
# "$" stays as it is), e.g. a password: ${RELAYHAT_PASSWORD}.
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

# stateFile keeps the state of every relay and its last switch (when, by which
# client), for startState: last and the "Last switch" in the web page. It is
# written after every switch and once on start; the directory must exist and be
# writable. Set it to "" to keep nothing (startState: last then needs a file).
stateFile: /opt/relayhat/data/state.yaml

# =============================================================================
# Webserver configuration (HTTPS)
# =============================================================================
webserver:
  # Host address the HTTPS server listens on (0.0.0.0 = all interfaces)
  listenHost: 0.0.0.0

  # Port the HTTPS server listens on
  listenPort: 8443

  # Bearer tokens accepted on the protected endpoints, one "<name> <sha256>" per line.
  # Manage it with "relayhat token add|remove|list"; a relative path is relative to this file.
  # The file holds hashes only, but whoever can write it can add a token: owner root.
  tokenFile: /opt/relayhat/etc/token-hashes

  # TLS private key file
  keyFile: /opt/relayhat/etc/key.pem

  # TLS certificate file
  certFile: /opt/relayhat/etc/cert.pem

  # Blocked IP addresses or networks (empty = none blocked)
  # An entry that is no address or network is a config error, also in allowedIPs.
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
# minSwitchInterval: after a switch the relay refuses to be switched again for
#   this long, by any client including the web page (HTTP 429), e.g. 5s or 10m;
#   0s or missing disables it. The start counts as a switch, start states are
#   never refused, and a request for the state the relay is already in is no
#   switch.
#
# Web page only (optional, the API paths stay /relays/{name}):
#   label:   name on the card, default the relay name
#   color:   pilot light when on: green (default) | red | amber
#   onText:  word for the on state, default ON
#   offText: word for the off state, default OFF
#
# gpio 4 and 17: Pi Zero Relay HAT and 4-Channel Relay HAT;
# gpio 22 and 27: 4-Channel Relay HAT only.
# =============================================================================
relay:
  relay1:
    label: "Heat pump"
    description: "Utility lock signal of the heat pump"
    gpio: 4
    startState: off
    minSwitchInterval: 5s
    color: red
    onText: "Locked"
    offText: "Released"
  relay2:
    description: "gpio 17, Pi Zero Relay HAT and 4-Channel Relay HAT"
    gpio: 17
  relay3:
    description: "gpio 22, 4-Channel Relay HAT only"
    gpio: 22
  relay4:
    description: "gpio 27, 4-Channel Relay HAT only"
    gpio: 27
```

---

## Tokens

Every caller gets its own token: a browser, Node-RED, a script. relayhat keeps only the SHA-256 hash
of each, in the file named by `webserver.tokenFile`, one line per caller:

```
# /opt/relayhat/etc/token-hashes
browser  3f2a…c91e
nodered  8b04…d7a2
```

The token itself exists only with the caller. Whoever reads the file cannot call relayhat with it,
so it needs no more protection than the config — but whoever can **write** it can add a token of
their own: keep it `root:relayhat 0640`, like `config.yaml`. relayhat logs the warning "Token file
is not protected" at every start and reload when it could change the file or a directory above it.

| Command | Does |
|---|---|
| `sudo relayhat token add <name>` | Writes the hash, then prints the token once, on stdout alone |
| `sudo relayhat token add <name> -dry-run` | Prints token and line, writes nothing |
| `sudo relayhat token remove <name>` | Deletes the line |
| `sudo relayhat token list` | Prints the names, never a hash |
| `sudo systemctl reload relayhat` | Applies a change; the relays keep their state |

`-config` works as for the service. A new token file gets `0640` and the group of `config.yaml`;
its owner is whoever runs the command, root with `sudo`. `add` refuses a name that is taken and
prints no token when it cannot write the file, so a token that is shown but stored nowhere cannot
happen. A name is 1–64 letters, digits, `.`, `_` or `-`.

**Rotating without a gap:** `relayhat token add nodered-2`, move the caller to the new token,
`relayhat token remove nodered`, reload. A lost token is not recovered but replaced the same way.

**In the log:** at start and after every reload relayhat names the tokens it accepts
(`Bearer tokens loaded … names=[browser nodered]`). Every switch is logged with `user`; the first
request of each caller from each address is logged at info (`user=browser request.client=…
firstAccess=true`), every rejected one as a warning with its address; the polls in between at debug.

---

## Security

- The REST API and the web page are served over HTTPS only, TLS 1.3. Use your own certificate and
  `env: prod` on every device, see [TLS Certificate](#tls-certificate).
- Give every caller its own token (`relayhat token add <name>`); everything except `/`, `/version`
  and the TLS handshake needs one. A token can switch every relay; relayhat stores only the hashes
  and records who switched.
- Keep `/opt/relayhat` owned by root except `data/`, as in the [Quick start](#quick-start), and run
  the unit from `contrib/` with its sandbox.
- Limit who can reach the port with `allowedIPs`, e.g. to your home network, and do not expose it
  to the internet.

Report vulnerabilities privately, see [SECURITY.md](SECURITY.md).

---

## Reload, restart and stop

Send `SIGHUP` to reload the configuration without restarting the process:

```sh
sudo systemctl reload relayhat
# or
kill -HUP $(pidof relayhat)
```

A reload also rereads the token file. The config file is validated first; if it is broken, the reload is refused, logged, and the service
keeps running unchanged. Relays whose GPIO is still configured keep their state across the reload
(also when they are renamed) — the GPIO line is handed over, not reopened, so the relay does not
flicker. Removed relays are switched off, new ones start in their `startState`. The last switch of a
relay is kept as well.

Some problems only show when the service starts with the new settings, e.g. a port or GPIO line in
use. Then `Start with the new configuration failed, continuing with the previous one` is logged and
the service starts again with the settings it ran with before; the relays are handed over to it and
keep their state. A request that arrives while a reload is in progress gets HTTP 503.

| Event                                            | Relay state afterwards              |
|--------------------------------------------------|-------------------------------------|
| `SIGHUP` reload                                  | unchanged                           |
| process start (boot, crash, `systemctl restart`) | `startState`: `off`, `on` or `last` |
| stop (`SIGTERM`/`SIGINT`, `systemctl stop`)      | off                                 |

A config that does not load or validate at start ends relayhat with exit code 78; the unit does not
restart it then (`RestartPreventExitStatus=78`), and `systemctl status relayhat` shows why.

---

## Command-line Flags

| Flag        | Default                         | Description                                       |
|-------------|---------------------------------|---------------------------------------------------|
| `--config`  | `/opt/relayhat/etc/config.yaml` | Path to the configuration file                    |
| `--debug`   | `false`                         | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                         | Print the application version and exit            |
| `--about`   | `false`                         | Print application details and exit                |
| `--help`    | `false`                         | Print this help message and exit                  |

`relayhat token add|remove|list` manages the bearer tokens, see [Tokens](#tokens).

The config file path can also be set via the environment variable `CONFIG_FILE`; `--config` wins
over it.

---

## TLS Certificate

relayhat serves HTTPS only. With `env: prod` it needs `certFile` and `keyFile`; the Quick start
creates a self-signed pair valid for 825 days, the maximum browsers accept. With `env: dev` it falls
back to a certificate embedded in the binary when `certFile` does not exist — convenient for a first
try, but that key ships in every release, so never use it on a reachable device.

---

## Backup & Restore

```sh
# Backup
sudo tar czf /tmp/relayhat-backup.tar.gz /opt/relayhat

# Restore
sudo tar xzf /tmp/relayhat-backup.tar.gz -C /     # as root, tar keeps owners and modes
sudo systemctl restart relayhat
```

---

## Releases

Every release on the [releases page](https://github.com/womat/relayhat/releases) carries archives for
all Raspberry Pi architectures with the binary, `config/config.yaml`, `contrib/relayhat.service`,
`README.md` and `LICENSE`,
plus a `checksums.txt` and a changelog. Versions follow
[semantic versioning](https://semver.org/); a breaking change of the API or the configuration raises
the major version.

`relayhat --version` reports the release a binary was built from. A local build reports something
like `2.0.1-3-g0c13781-dirty` instead, which is how the two are told apart on a device.

Building from source needs Go and `make`: clone the repository and run `make help` for the targets;
`make test` (the tests, Linux only) and `make lint` (go vet, golangci-lint, govulncheck) run what CI
checks. [`CLAUDE.md`](CLAUDE.md) describes the architecture, the tests and the release process.

### Upgrading to 2.0.0

The API key is replaced by bearer tokens, `lastChange.host` by `lastChange.user`, HTTPS is TLS 1.3
only, and the unit lives in the repository:

1. Make `/opt/relayhat` root-owned except `data/`, as in the [Quick start](#quick-start).
2. Put the new binary next to the old one as `bin/relayhat.new`, and a copy of the config as
   `etc/config.yaml.new` with `apiKey: …` replaced by `tokenFile: token-hashes`. The running 1.x is not
   touched by either.
3. Create the tokens with the new binary, one per caller: `sudo /opt/relayhat/bin/relayhat.new
   token add nodered -config /opt/relayhat/etc/config.yaml.new`. This only writes the token file;
   it opens no GPIO.
4. Move `relayhat.new` and `config.yaml.new` into place, install `contrib/relayhat.service`, then
   `sudo systemctl daemon-reload && sudo systemctl restart relayhat`. Like every restart, this
   switches the relays off for a moment; with `startState: last` they come back. A config that
   still has `apiKey` is refused with a message that says so.
5. Every caller sends `Authorization: Bearer <token>`; `X-API-Key` is no longer accepted. In
   Node-RED, set the *http request* nodes to *bearer authentication*. The web page asks for the
   token once.
6. A client that told its own switches by `lastChange.host` uses `lastChange.user` now;
   `lastChange.client` stays. The state file of 1.x is read, its `host` ignored.
7. A client limited to TLS 1.2 can no longer connect.

---

## License

relayhat is released under the MIT License - see [`LICENSE`](LICENSE) for the full text.

### Third-party licenses

The source tree contains no third-party code, but a **compiled binary statically links** the
modules below. Their terms apply to anyone distributing that binary, not to the sources here.

| Module                                            | License                |
|---------------------------------------------------|------------------------|
| `github.com/womat/golib/v2`                       | MIT                    |
| `github.com/womat/gpio`                           | MIT                    |
| `github.com/warthog618/go-gpiocdev`               | MIT                    |
| `gopkg.in/yaml.v3`                                | MIT and Apache-2.0     |
| `golang.org/x/sys`                                | BSD-3-Clause           |
| Swagger UI build only (`-tags swagger`):          |                        |
| `github.com/swaggo/swag`, `http-swagger`, `files` | MIT                    |
| `github.com/go-openapi/*`, `go.yaml.in/yaml/v3`   | Apache-2.0             |
| `github.com/KyleBanks/depth`                      | MIT                    |
| `golang.org/x/net`, `mod`, `sync`, `tools`        | BSD-3-Clause           |

All of these are permissive; none obliges relayhat to change its license.
