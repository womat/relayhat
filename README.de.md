# relayhat – Deutsche Kurzfassung

🇬🇧 [Full documentation in English](README.md)

<p align="center">
  <img src="docs/screenshots/web-ui.png" width="640" alt="Weboberfläche von relayhat mit zwei Relais">
</p>

**relayhat schaltet die Relais eines Raspberry-Pi-Relay-HATs aus dem Browser, aus Node-RED oder aus
Home Assistant.**

Ein Relay-HAT macht aus einem Raspberry Pi einen Schalter für Dinge, die der Pi selbst nicht treiben
kann: die EVU-Sperre oder den PV-Überschuss-Eingang einer Wärmepumpe, eine Pumpe, ein Tor, eine
Lampe. relayhat macht diese Relais im ganzen Heimnetz nutzbar:

- über eine **REST-API** per HTTPS (TLS 1.3) mit einem eigenen Token je Aufrufer, z. B. für Node-RED,
  Home Assistant, ioBroker oder ein `curl` im Skript; relayhat speichert davon nur den SHA-256-Hash
  und merkt sich, mit welchem Token zuletzt geschaltet wurde,
- über eine **eingebaute Webseite** mit Meldeleuchte pro Relais und einem **Schalten in zwei
  Schritten**, das ein versehentlicher Tipp nicht auslöst,
- die Relais **behalten ihren Zustand**, wenn die Konfiguration neu geladen wird, und kommen nach
  Neustart oder Stromausfall im gewünschten Zustand zurück (`off`, `on` oder der **letzte**),
- eine **Schaltsperre** pro Relais gegen schnelles Hin- und Herschalten, und die Anzeige, **wer
  zuletzt geschaltet hat**.

Keine Cloud, keine Datenbank: ein einzelnes Programm und eine YAML-Datei.

## In fünf Schritten

1. **Herunterladen:** Das Archiv für deinen Pi gibt es unter
   [Releases](https://github.com/womat/relayhat/releases/latest): `armv6` für Pi 1 und Zero (läuft
   auf jedem Pi), `armv7` für 32-Bit-Systeme, `arm64` für 64-Bit-Systeme.
2. **Installieren:** System-User `relayhat` anlegen, Programm und `config.yaml` nach `/opt/relayhat`
   kopieren, Zertifikat erzeugen. Alles gehört root außer `data/` mit den Relais-Zuständen, der
   Dienst darf `etc/` nur lesen – wer Konfiguration oder Token-Datei schreiben kann, kann sich
   selbst einen Token eintragen und damit schalten.
3. **Konfigurieren:** `env: prod` und einen Eintrag pro Relais: GPIO, Startzustand, auf Wunsch
   Schaltsperre, Anzeigename, Farbe und eigene Wörter für ein/aus (z. B. „Gesperrt“/„Freigegeben“).
   Dann je Aufrufer einen Token anlegen, z. B. `sudo /opt/relayhat/bin/relayhat token add nodered`;
   er wird nur dieses eine Mal angezeigt, siehe [Tokens](README.md#tokens).
4. **Anschließen:** HAT aufstecken. Der 2-Kanal-HAT für den Pi Zero nutzt GPIO 4 und 17, der
   4-Kanal-HAT GPIO 4, 17, 27 und 22. **Netzspannung (230 V) gehört in die Hände einer
   Elektrofachkraft.** Im Zweifel einen Schütz oder einen Steuereingang schalten, nicht die Last
   selbst.
5. **Starten:** mit der Unit aus dem Archiv (`contrib/relayhat.service`, läuft abgeschottet), dann
   `https://<dein-pi>:8443/` im Browser öffnen und den Token eingeben.

**Von 1.x auf 2.0.0:** `apiKey` gibt es nicht mehr, stattdessen `tokenFile` und
`relayhat token add`; `lastChange.host` ist durch `lastChange.user` ersetzt, HTTPS nur noch mit
TLS 1.3. Die Schritte stehen unter [Upgrading to 2.0.0](README.md#upgrading-to-200).

Die genauen Befehle stehen im [Quick start](README.md#quick-start), alle Einstellungen unter
[Configuration](README.md#configuration), Beispiele für Node-RED und Home Assistant unter
[Node-RED and Home Assistant](README.md#node-red-and-home-assistant).

## Lizenz

MIT, siehe [`LICENSE`](LICENSE).
