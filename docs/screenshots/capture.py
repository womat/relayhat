"""Check the relayhat web UI in a real browser and capture the README screenshots.

Serves the real app/ui/index.html with a mocked API (/relays, /health and PATCH
/relays/{name}/{state}, API key "demo"), clicks through login, the two-step switch and a
rejected key in headless Chromium, then photographs the page. Run from the project root, on
demand only:

    docker run --rm -v "$PWD":/src -w /src mcr.microsoft.com/playwright/python:v1.52.0-noble \
        sh -c 'pip install -q playwright==1.52.0 && python3 docs/screenshots/capture.py'

Writes docs/screenshots/web-ui*.png. Fails with an AssertionError when the page misbehaves.
"""

import copy
import json
import threading
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[2]
PAGE = (ROOT / "app/ui/index.html").read_bytes()
OUT = ROOT / "docs/screenshots"
TZ = timezone(timedelta(hours=2))
PORT = 8765
KEY = "demo"
UPTIME = 3 * 86400 + 4 * 3600 + 12 * 60

# The relays of a heat pump: an EVU lock and a forced run on PV surplus. "age" is the seconds
# since the last switch.
RELAYS = [
    dict(name="relay1", state="off", description="Sperrt die Wärmepumpe (EVU-Sperrsignal)", gpio=4,
         display=dict(label="EVU", color="red", onText="Gesperrt", offText="Freigegeben"),
         age=26 * 3600 + 7 * 60, source="api", client="192.168.65.20", host="nodered.fritz.box"),
    dict(name="relay2", state="on", description="Zwangsbetrieb bei PV-Überschuss", gpio=17,
         display=dict(label="Überschusssteuerung", color="green", onText="Zwangsbetrieb", offText="Normal"),
         age=47 * 60, source="api", client="192.168.65.20", host="nodered.fritz.box"),
]


class Api:
    """The mocked relayhat state, shared by the handler threads."""

    def __init__(self):
        self.lock = threading.Lock()
        self.reset()

    def reset(self):
        with self.lock:
            now = datetime.now(TZ)
            self.relays = copy.deepcopy(RELAYS)
            for r in self.relays:
                r["changed"] = now - timedelta(seconds=r.pop("age"))
            self.patches = []
            self.reject = False

    def view(self, r):
        now = datetime.now(TZ)
        out = {k: r[k] for k in ("name", "state", "description", "gpio", "display")}
        out["lastChange"] = {
            "time": r["changed"].replace(microsecond=0).isoformat(),
            "ageSeconds": (now - r["changed"]).total_seconds(),
            "source": r["source"], "client": r["client"], "host": r["host"],
        }
        return out


API = Api()


class Handler(BaseHTTPRequestHandler):
    def send_json(self, code, body):
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(data)

    def authorized(self):
        if API.reject or self.headers.get("X-API-Key") != KEY:
            self.send_json(401, {"error": "not authorized"})
            return False
        return True

    def do_GET(self):
        if self.path == "/":
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.end_headers()
            self.wfile.write(PAGE)
            return
        if not self.authorized():
            return
        with API.lock:
            if self.path == "/relays":
                self.send_json(200, [API.view(r) for r in API.relays])
            elif self.path == "/health":
                self.send_json(200, {"app": "relayhat", "appVersion": "1.8.0", "hostname": "heatpump",
                                     "uptimeSeconds": UPTIME})
            else:
                self.send_json(404, {"error": "not found"})

    def do_PATCH(self):
        if not self.authorized():
            return
        parts = self.path.strip("/").split("/")
        with API.lock:
            r = next((r for r in API.relays if len(parts) == 3 and r["name"] == parts[1]), None)
            if r is None:
                self.send_json(404, {"error": "relay not found"})
                return
            if parts[2] not in ("on", "off"):
                self.send_json(400, {"error": "invalid state: must be \"on\" or \"off\""})
                return
            API.patches.append((r["name"], parts[2]))
            r.update(state=parts[2], changed=datetime.now(TZ), source="api", client="192.168.65.23",
                     host="macbook.fritz.box")
            self.send_json(200, API.view(r))

    def log_message(self, *args):
        pass


def check(browser):
    """Click through the page and assert what it does."""
    API.reset()
    page = browser.new_page(viewport={"width": 1100, "height": 800})
    page.goto(f"http://localhost:{PORT}/")
    card = page.locator("article.card").first
    button = card.locator("button.switch")

    # Login: a wrong key is rejected, the right one shows the relays.
    assert page.locator("#login").is_visible(), "no login without a stored key"
    page.fill("#apikey", "wrong")
    page.click("#keyform button")
    page.wait_for_selector("#keyerr:not([hidden])")
    page.fill("#apikey", KEY)
    page.click("#keyform button")
    page.wait_for_selector("article.card")
    assert page.locator("article.card").count() == 2
    assert card.locator("h2").inner_text() == "EVU"
    assert card.locator(".gpio").inner_text() == "GPIO 4"
    assert card.locator(".val").inner_text() == "Freigegeben"
    assert button.inner_text() == "→ Gesperrt"
    assert card.locator(".by").inner_text() == "nodered"

    # First tap arms, nothing is switched; the button falls back after 3 s.
    button.click()
    assert "armed" in card.get_attribute("class")
    assert button.inner_text() == "Confirm: Gesperrt"
    page.wait_for_timeout(3300)
    assert "armed" not in card.get_attribute("class"), "the button did not disarm"
    assert not API.patches, "the first tap switched"

    # A fast double tap only arms; Esc disarms.
    button.dblclick()
    assert "armed" in card.get_attribute("class")
    page.keyboard.press("Escape")
    assert "armed" not in card.get_attribute("class"), "Esc did not disarm"
    assert not API.patches, "a double tap switched"

    # Tap, wait, tap: switched.
    button.click()
    page.wait_for_timeout(500)
    button.click()
    page.wait_for_function("document.querySelector('article.card .val').textContent === 'Gesperrt'")
    assert API.patches == [("relay1", "on")], API.patches
    assert "on" in card.get_attribute("class").split()
    assert card.locator(".by").inner_text() == "macbook"

    # A key the server no longer accepts leads back to the login.
    API.reject = True
    page.wait_for_selector("#keyerr:not([hidden])", timeout=5000)
    page.close()
    print("check passed")


def shoot(browser, path, width, height, scheme, scale=1, full_page=True):
    API.reset()
    ctx = browser.new_context(viewport={"width": width, "height": height}, timezone_id="Europe/Vienna",
                              color_scheme=scheme, device_scale_factor=scale)
    ctx.add_init_script(f"localStorage.setItem('relayhat.apiKey', '{KEY}')")
    page = ctx.new_page()
    page.goto(f"http://localhost:{PORT}/")
    page.wait_for_selector("article.card")
    # Show the confirmation step on the EVU card.
    page.locator("article.card button.switch").first.click()
    page.screenshot(path=str(path), full_page=full_page, animations="disabled")
    ctx.close()
    print("wrote", path.relative_to(ROOT))


def main():
    server = ThreadingHTTPServer(("localhost", PORT), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    OUT.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as p:
        browser = p.chromium.launch()
        check(browser)
        shoot(browser, OUT / "web-ui.png", 1100, 520, "light")
        shoot(browser, OUT / "web-ui-dark.png", 1100, 520, "dark")
        # Phone: the first screen only, so it sits next to the desktop shot in the README.
        shoot(browser, OUT / "web-ui-phone.png", 390, 760, "light", scale=2, full_page=False)
        browser.close()
    server.shutdown()


if __name__ == "__main__":
    main()
