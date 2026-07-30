# ---- Build-Stage: statisches Go-Binary (kein cgo dank modernc.org/sqlite) ----
FROM golang:1.24-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /startseite .

# ---- Runtime-Stage: Chromium via headless-shell (für chromedp) ----
FROM chromedp/headless-shell:latest

# CA-Zertifikate aus der Build-Stage (für HTTPS-Metadaten-Abruf); kein apt nötig.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENV DATA_DIR=/data
ENV PORT=3000
ENV CHROME_PATH=/headless-shell/headless-shell

COPY --from=builder /startseite /usr/local/bin/startseite

EXPOSE 3000
ENTRYPOINT ["startseite"]
