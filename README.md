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
- ↕️ **Drag & Drop** — Links sortieren und per Ziehen in andere Kategorien verschieben
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
├── app.db            # SQLite-Datenbank (Links, Kategorien)
├── app.db-wal        # WAL-Journal
└── thumbnails/       # generierte Screenshot-Vorschauen (*.jpg)
```

## Schnellstart (Docker)

1. **Secret erzeugen und Image bauen:**

   ```bash
   openssl rand -hex 32                 # ergibt das SESSION_SECRET
   docker compose build
   ```

2. **Passwort-Hash erzeugen** (das Binary kann das selbst):

   ```bash
   docker compose run --rm startseite -hash "meinGeheimesPasswort"
   ```

3. **`.env` anlegen** (neben der `docker-compose.yml`):

   ```env
   APP_PASSWORD_HASH=$2a$12$....      # Ausgabe aus Schritt 2
   SESSION_SECRET=....                # Ausgabe aus "openssl rand -hex 32"
   ```

4. **Starten:**

   ```bash
   docker compose up -d
   ```

   Dashboard: <http://localhost:8080> — die Daten landen im Ordner `./data`.

## Lokale Entwicklung (ohne Docker)

Voraussetzung: Go 1.23+ und ein installiertes Chrome/Chromium.

```bash
go run . -hash "test"                 # Hash ausgeben, in .env eintragen
export APP_PASSWORD_HASH='...' SESSION_SECRET='dev' DATA_DIR=./data PORT=8090
# macOS: falls Chrome nicht gefunden wird:
# export CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
go run .
```

Dann <http://localhost:8090> öffnen.

## Konfiguration (Umgebungsvariablen)

| Variable            | Bedeutung                                                        |
| ------------------- | --------------------------------------------------------------- |
| `APP_PASSWORD_HASH` | bcrypt-Hash des Master-Passworts (Pflicht)                     |
| `SESSION_SECRET`    | Secret zum Signieren des Session-Cookies (Pflicht)            |
| `DATA_DIR`          | Datenverzeichnis; im Container `/data`, lokal z. B. `./data`   |
| `PORT`              | HTTP-Port (Standard 3000)                                      |
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
