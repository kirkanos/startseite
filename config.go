package main

import (
	"os"
	"path/filepath"
)

// Config bündelt alle Einstellungen aus den Umgebungsvariablen.
type Config struct {
	PasswordHash string // bcrypt-Hash des Master-Passworts
	SessionKey   []byte // Secret zum Signieren des Session-Cookies
	DataDir      string // Verzeichnis für DB + Thumbnails
	ThumbDir     string // = DataDir/thumbnails
	Port         string // HTTP-Port
	ChromePath   string // optionaler Pfad zum Chrome/Chromium-Binary
}

func loadConfig() Config {
	dataDir := env("DATA_DIR", "/data")
	return Config{
		PasswordHash: os.Getenv("APP_PASSWORD_HASH"),
		SessionKey:   []byte(env("SESSION_SECRET", "dev-insecure-secret-please-change")),
		DataDir:      dataDir,
		ThumbDir:     filepath.Join(dataDir, "thumbnails"),
		Port:         env("PORT", "3000"),
		ChromePath:   os.Getenv("CHROME_PATH"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
