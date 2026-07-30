# Startseite

Ein selbstgehostetes Link-Dashboard: Websites mit Screenshot-Vorschau und Kategorien
sammeln, über ein Webinterface pflegen, mit Passwortschutz und „Angemeldet bleiben".
Läuft als einzelner Docker-Container; alle Daten liegen in einem **Host-Verzeichnis**.

Geschrieben in **Go** — ein einzelnes statisches Binary, **kein Node, kein cgo**,
keine nativen Kompilier-Abhängigkeiten.

## Funktionen

- 🔒 **Passwortschutz** für das gesamte Dashboard (ein Master-Passwort)
- 🍪 **„Angemeldet bleiben"** — HMAC-signiertes Session-Cookie, 30 Tage
- 🖼️ **Echte Screenshot-Vorschau** jeder Seite (Chromium via chromedp)
- 🗂️ **Kategorien** mit Farben — anlegen, umbenennen, umfärben, löschen
- ↕️ **Drag & Drop** — Links sortieren und per Ziehen in andere Kategorien verschieben;
  Kategorien selbst über den Ziehgriff in „Kategorien verwalten" umsortieren
- 🎛️ **Ansicht konfigurierbar** („Ansicht" in der Kategorie-Zeile) — Spaltenzahl
  (automatisch oder 2–6), Kartenbreite und Screenshot-Vorschauen ein/aus.
  Alles liegt **in der Datenbank**, gilt also auf jedem Rechner gleich
- ✏️ **Links bearbeiten** — URL, Titel, Kategorie; Screenshot optional neu erzeugen
- 🌐 **Öffentliche Links** — einzeln als „öffentlich" markierbar; ohne Login sichtbar
- 🚪 **Login optional** — die Einstiegsseite `/` zeigt ohne Login die öffentlichen Links, nach dem Login das volle Dashboard
- 🌍 **Deutsch / Englisch** — Umschalter „DE | EN" in der Kopfzeile; die Wahl wird im Cookie gemerkt
- 🔎 Suche und Kategorie-Filter (clientseitig, ohne Reload)
- 💾 Daten (SQLite + Thumbnails) in einem **Host-Directory**, kein Docker-Volume

## Technik

| Bereich    | Wahl                                                          |
| ---------- | ------------------------------------------------------------ |
| Sprache    | Go (net/http, html/template — Standardbibliothek)            |
| Datenbank  | SQLite über `modernc.org/sqlite` (pure Go, kein cgo)         |
| Auth       | Master-Passwort (bcrypt) + HMAC-signiertes Cookie            |
| Thumbnails | `chromedp` (Chrome DevTools Protocol) → JPEG in `data/thumbnails/` |
| Deployment | Docker (Multi-Stage), Runtime auf `chromedp/headless-shell`  |

## Datenablage

```
data/
├── app.db            # SQLite-Datenbank (Links, Kategorien, Ansichts-Einstellungen)
├── app.db-wal        # WAL-Journal
└── thumbnails/       # generierte Screenshot-Vorschauen (*.jpg)
```

## Schnellstart (Docker, hinter Traefik)

Die `docker-compose.yml` ist für den Betrieb hinter einem **Traefik**-Reverse-Proxy
ausgelegt (externes Netzwerk `traefik-network`, TLS via `certresolver=default`).

1. **`.env` aus der Vorlage anlegen:**

   ```bash
   cp .env.sample .env
   ```

2. **Secret und Passwort-Hash erzeugen** und in die `.env` eintragen:

   ```bash
   openssl rand -hex 32                              # -> SESSION_SECRET
   docker compose run --rm web -hash "meinPasswort"  # -> APP_PASSWORD_HASH
   ```

   `SERVICE`, `HOST` und `PORT` in der `.env` an die eigene Umgebung anpassen.

3. **Starten:**

   ```bash
   docker compose up -d --build
   ```

   Erreichbar unter `https://$HOST`. Die Daten (SQLite + Thumbnails) landen im
   Host-Ordner `./data`.

### Nur lokal testen (ohne Traefik)

```bash
docker build -t startseite .
docker run --rm -p 8080:3000 \
  -e APP_PASSWORD_HASH="$(docker run --rm startseite -hash test)" \
  -e SESSION_SECRET=dev -v "$PWD/data:/data" startseite
# -> http://localhost:8080
```

## Lokale Entwicklung (ohne Docker)

Voraussetzung: Go 1.23+ und ein installiertes Chrome/Chromium.

```bash
go run . -hash "test"                 # Hash ausgeben, unten einsetzen
export APP_PASSWORD_HASH='...' SESSION_SECRET='dev' DATA_DIR=./data PORT=8090
# macOS: falls Chrome nicht gefunden wird:
# export CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
go run .
```

Dann <http://localhost:8090> öffnen.

## Deployment (Woodpecker CI + sops)

Die Secrets liegen **verschlüsselt** als [`.env.enc`](.env.enc) im Repo (sops + age);
die Klartext-`.env` ist per `.gitignore` ausgeschlossen.

`.env` verschlüsseln (age-Key aus `$SOPS_AGE_KEY_FILE`):

```bash
sops --encrypt --age "$(cat "$SOPS_AGE_KEY_FILE" | ggrep -oP 'public key: \K(.*)')" \
  --input-type dotenv --output-type dotenv --output .env.enc .env
```

Die Pipeline [`.woodpecker/pipeline.yaml`](.woodpecker/pipeline.yaml) läuft bei Push
auf `main` und

1. entschlüsselt `.env.enc` → `.env` (sops, age-Key aus Secret `sops_age_key`),
2. lädt den Build-Kontext (Go-Quellen, `templates/`, `static/`, Compose, `.env`)
   per `scp` nach `/services/$SERVICE/` auf `ssh_host_local` und installiert die
   systemd-Unit aus `template.service`,
3. startet den Dienst neu — der Container wird **auf dem Zielhost** gebaut
   (`docker compose up --build --pull always` via systemd).

Benötigte Woodpecker-Secrets: `sops_age_key` (privater age-Key) und `ssh_host_local`
(Ziel `user@host` für SSH auf Port 822).

## Konfiguration (Umgebungsvariablen)

| Variable            | Bedeutung                                                        |
| ------------------- | --------------------------------------------------------------- |
| `SERVICE`           | Dienstname (Container, Traefik-Router, Deploy-Verzeichnis)      |
| `HOST`              | öffentliche Domain für Traefik (z. B. `start.kirkanos.net`)     |
| `APP_PASSWORD_HASH` | bcrypt-Hash des Master-Passworts (Pflicht)                     |
| `SESSION_SECRET`    | Secret zum Signieren des Session-Cookies (Pflicht)            |
| `DATA_DIR`          | Datenverzeichnis; im Container `/data`, lokal z. B. `./data`   |
| `PORT`              | HTTP-Port (Standard 3000; = Traefik-Ziel-Port)                 |
| `CHROME_PATH`       | Pfad zum Browser-Binary (im Container gesetzt; lokal optional) |

## Hinweise

- **Kein CSRF-Sonderfall:** Die Formulare funktionieren unter jeder Adresse
  (localhost, LAN-IP, Hostname) ohne Konfiguration. Der Schutz kommt vom
  `SameSite=lax`-Session-Cookie — bei Cross-Site-POSTs schickt der Browser das
  Cookie nicht mit, und alle ändernden Aktionen sind POST.
- Hinter einem HTTPS-Reverse-Proxy setzt der Proxy den Header `X-Forwarded-Proto:
  https`; dann werden die Cookies automatisch als `Secure` markiert.
- Passwort ändern = neuen Hash erzeugen, `APP_PASSWORD_HASH` aktualisieren, Container neu starten.

## Lizenz

Lizenziert unter der [Apache License 2.0](LICENSE).

© 2026 [kirkanos](https://www.kirkanos.net)
