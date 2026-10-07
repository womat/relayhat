"""Check the relayhat web UI in a real browser and capture the README screenshots and social preview.

Serves the real app/ui/index.html with a mocked API (/relays, /health and PATCH
/relays/{name}/{state}, API key "demo"), clicks through login, the two-step switch and a
rejected key in headless Chromium, then photographs the page. Run from the project root, on
demand only:

    docker run --rm -v "$PWD":/src -w /src mcr.microsoft.com/playwright/python:v1.52.0-noble \
        sh -c 'pip install -q playwright==1.52.0 && python3 docs/screenshots/capture.py'

Writes docs/screenshots/web-ui*.png and docs/social-preview.png. Fails with an AssertionError when
the page misbehaves.
"""

import base64
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

# The relays of a heat pump: the utility's lock signal and a forced run on PV surplus. "age" is the
# seconds since the last switch. Host names and addresses are examples, not a real network.
RELAYS = [
    dict(name="relay1", state="off", description="Utility lock signal of the heat pump", gpio=4,
         display=dict(label="Utility lock", color="red", onText="Locked", offText="Released"),
         age=26 * 3600 + 7 * 60, source="api", client="192.168.1.20", host="nodered.lan"),
    dict(name="relay2", state="on", description="Forced run on PV surplus", gpio=17,
         display=dict(label="PV surplus", color="green", onText="Forced run", offText="Normal"),
         age=47 * 60, source="api", client="192.168.1.20", host="nodered.lan", lock=5),
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

    @staticmethod
    def locked(r, now):
        """Seconds the relay's switch lock still runs, like relayhat's lock.remainingSeconds."""
        if not r.get("lock"):
            return 0
        return max(0, r["lock"] - (now - r["changed"]).total_seconds())

    def view(self, r):
        now = datetime.now(TZ)
        out = {k: r[k] for k in ("name", "state", "description", "gpio", "display")}
        if r.get("lock"):
            out["lock"] = {"intervalSeconds": r["lock"], "remainingSeconds": self.locked(r, now)}
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
                self.send_json(200, {"app": "relayhat", "appVersion": "1.8.0", "hostname": "pi-heating",
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
            if r["state"] == parts[2]:
                self.send_json(200, API.view(r))
                return
            left = API.locked(r, datetime.now(TZ))
            if left > 0:
                self.send_json(429, {"error": f"switching locked for {int(left) + 1}s (minSwitchInterval {r['lock']}s)"})
                return
            API.patches.append((r["name"], parts[2]))
            r.update(state=parts[2], changed=datetime.now(TZ), source="api", client="192.168.1.23",
                     host="laptop.lan")
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
    assert card.locator("h2").inner_text() == "Utility lock"
    assert card.locator(".gpio").inner_text() == "GPIO 4"
    assert card.locator(".val").inner_text() == "Released"
    assert button.inner_text() == "→ Locked"
    assert card.locator(".by").inner_text() == "nodered"

    # First tap arms, nothing is switched; the button falls back after 3 s.
    button.click()
    assert "armed" in card.get_attribute("class")
    assert button.inner_text() == "Confirm: Locked"
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
    page.wait_for_function("document.querySelector('article.card .val').textContent === 'Locked'")
    assert API.patches == [("relay1", "on")], API.patches
    assert "on" in card.get_attribute("class").split()
    assert card.locator(".by").inner_text() == "laptop"

    # The second relay has a 5 s switch lock: after a switch its button is locked and counts down.
    card2 = page.locator("article.card").nth(1)
    button2 = card2.locator("button.switch")
    assert card2.locator(".lockinfo").inner_text() == "Switch lock: 5 s after each switch"
    button2.click()
    page.wait_for_timeout(400)
    button2.click()
    page.wait_for_function("document.querySelectorAll('article.card')[1].querySelector('.switch').disabled")
    assert API.patches[-1] == ("relay2", "off"), API.patches
    assert button2.inner_text().startswith("🔒 Locked · 0:0"), button2.inner_text()
    button2.click(force=True)
    assert len(API.patches) == 2, "a locked button switched"
    page.wait_for_function("!document.querySelectorAll('article.card')[1].querySelector('.switch').disabled", timeout=8000)
    assert button2.inner_text() == "→ Forced run", button2.inner_text()

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
    # Show the confirmation step on the utility lock card.
    page.locator("article.card button.switch").first.click()
    page.screenshot(path=str(path), full_page=full_page, animations="disabled")
    ctx.close()
    print("wrote", path.relative_to(ROOT))


def social(browser):
    """docs/social-preview.png, 1280x640, the image GitHub shows when the repository is shared."""
    shot = base64.b64encode((OUT / "web-ui.png").read_bytes()).decode()
    html = f"""<!doctype html><meta charset="utf-8">
<style>
  body {{ margin: 0; width: 1280px; height: 640px; background: #f3f5f7; font-family: system-ui, sans-serif;
         display: flex; align-items: center; gap: 56px; padding: 0 0 0 96px; box-sizing: border-box; overflow: hidden; }}
  .text {{ display: flex; flex-direction: column; gap: 22px; width: 470px; flex: none; }}
  .brand {{ display: flex; align-items: center; gap: 22px; }}
  svg {{ width: 104px; height: 78px; color: #2563a8; }}
  h1 {{ margin: 0; font-size: 84px; letter-spacing: -.02em; color: #17202b; }}
  h1 b {{ color: #2563a8; }}
  p {{ margin: 0; font-size: 30px; line-height: 1.3; color: #3d4a58; }}
  .tags {{ font-size: 21px; color: #5d6b7a; }}
  img {{ height: 520px; border-radius: 14px; box-shadow: 0 20px 50px rgba(23, 32, 43, .18);
         border: 1px solid #dde2e8; object-fit: cover; object-position: left top; width: 900px; }}
</style>
<div class="text">
  <div class="brand"><svg viewBox="0 0 40 30" fill="none" stroke="currentColor" stroke-width="2.4"
    stroke-linecap="round" stroke-linejoin="round"><path d="M2 21h9"/><circle cx="13" cy="21" r="2"/>
    <path d="M14.8 20 27 12"/><circle cx="29" cy="21" r="2"/><path d="M31 21h7"/><path d="M20 4v9" stroke-dasharray="2 3"/></svg>
    <h1><b>relay</b>hat</h1></div>
  <p>Switch the relays of a Raspberry Pi relay HAT from the browser, Node-RED or Home Assistant.</p>
  <span class="tags">REST API · web page · two-step switching</span>
</div>
<img src="data:image/png;base64,{shot}" alt="">"""
    page = browser.new_page(viewport={"width": 1280, "height": 640})
    page.set_content(html)
    path = ROOT / "docs/social-preview.png"
    page.screenshot(path=str(path))
    page.close()
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
        social(browser)
        browser.close()
    server.shutdown()


if __name__ == "__main__":
    main()
