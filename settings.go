// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// Settings sind die Ansichts-Einstellungen des Dashboards. Sie liegen in der
// Datenbank (Tabelle "settings") und nicht im Browser — damit dieselbe Ansicht
// auf jedem Rechner erscheint.
type Settings struct {
	Columns   string `json:"columns"`    // "auto" oder "2".."6"
	CardSize  string `json:"card_size"`  // "s" | "m" | "l" (Kartenbreite bei "auto")
	CardStyle string `json:"card_style"` // "thumbs" | "compact" | "tile"
	Theme     string `json:"theme"`      // "auto" | "light" | "dark" | "dashy"
	ShowNSFW  bool   `json:"show_nsfw"`  // NSFW-Kategorien im Dashboard einblenden
	UncatSpan int    `json:"-"`          // Breite der Sektion "Ohne Kategorie"
}

func defaultSettings() Settings {
	return Settings{Columns: "auto", CardSize: "m", CardStyle: "thumbs", Theme: "auto", ShowNSFW: false, UncatSpan: maxSpan}
}

// Kategorie-Sektionen liegen in einem 12-Spalten-Raster. minSpan verhindert
// unbrauchbar schmale Sektionen.
const (
	minSpan = 3
	maxSpan = 12
)

func clampSpan(n int) int {
	if n < minSpan {
		return minSpan
	}
	if n > maxSpan {
		return maxSpan
	}
	return n
}

// sectionCols verteilt eine feste Spaltenzahl anteilig auf eine Sektion, damit
// die Karten über die ganze Seite gleich breit bleiben. 0 = automatisch.
func sectionCols(columns string, span int) int {
	n, err := strconv.Atoi(columns)
	if err != nil {
		return 0
	}
	c := (n*span + maxSpan/2) / maxSpan // kaufmännisch gerundet
	if c < 1 {
		c = 1
	}
	return c
}

// ThemeAttr liefert den Wert für das data-theme-Attribut am <html>. "auto"
// wird zu einem leeren Attribut — dann entscheidet prefers-color-scheme.
func (s Settings) ThemeAttr() string {
	if s.Theme == "auto" {
		return ""
	}
	return s.Theme
}

// NSFWAttr liefert den Wert für das data-nsfw-Attribut am <body>. "off"
// blendet NSFW-Kategorien per CSS aus — Chips wie Sektionen.
func (s Settings) NSFWAttr() string {
	return onOff(s.ShowNSFW)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

var (
	validColumns    = map[string]bool{"auto": true, "2": true, "3": true, "4": true, "5": true, "6": true}
	validSizes      = map[string]bool{"s": true, "m": true, "l": true}
	validCardStyles = map[string]bool{"thumbs": true, "compact": true, "tile": true}
)

// sanitize ersetzt unbekannte Werte durch die Standardwerte.
func (s Settings) sanitize() Settings {
	d := defaultSettings()
	if !validColumns[s.Columns] {
		s.Columns = d.Columns
	}
	if !validSizes[s.CardSize] {
		s.CardSize = d.CardSize
	}
	if !validCardStyles[s.CardStyle] {
		s.CardStyle = d.CardStyle
	}
	if !validTheme(s.Theme) {
		s.Theme = d.Theme
	}
	s.UncatSpan = clampSpan(s.UncatSpan)
	return s
}

// loadSettings liest die Einstellungen; fehlende Schlüssel bleiben auf Standard.
func loadSettings(db *sql.DB) Settings {
	s := defaultSettings()
	hasStyle, oldCompact := false, false
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return s
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return s
		}
		switch k {
		case "columns":
			s.Columns = v
		case "card_size":
			s.CardSize = v
		case "card_style":
			s.CardStyle, hasStyle = v, true
		case "theme":
			// "dashy" hieß die erste, handgeschriebene Nord-Palette; sie ist
			// in der übertragenen Themepalette als nord-frost aufgegangen.
			if v == "dashy" {
				v = "nord-frost"
			}
			s.Theme = v
		case "show_thumbs":
			// Vorgänger von "card_style": nur noch als Wanderung gelesen.
			if v == "0" {
				oldCompact = true
			}
		case "show_nsfw":
			s.ShowNSFW = v == "1"
		case "uncat_span":
			if n, err := strconv.Atoi(v); err == nil {
				s.UncatSpan = n
			}
		}
	}
	if oldCompact && !hasStyle {
		s.CardStyle = "compact"
	}
	return s.sanitize()
}

// saveSettings schreibt nur die Werte aus dem Ansichts-Panel — die Breite der
// Sektion "Ohne Kategorie" hat einen eigenen Weg (setSetting).
func saveSettings(db *sql.DB, s Settings) error {
	pairs := [][2]string{
		{"columns", s.Columns},
		{"card_size", s.CardSize},
		{"card_style", s.CardStyle},
		{"theme", s.Theme},
		{"show_nsfw", boolStr(s.ShowNSFW)},
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range pairs {
		if _, err := tx.Exec(upsertSetting, p[0], p[1]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const upsertSetting = `INSERT INTO settings (key, value) VALUES (?, ?)
                       ON CONFLICT(key) DO UPDATE SET value = excluded.value`

func setSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(upsertSetting, key, value)
	return err
}

// handleSaveSettings speichert die Ansichts-Einstellungen (JSON-Body).
func (a *App) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&s); err != nil {
		http.Error(w, "Ungültige Daten", http.StatusBadRequest)
		return
	}
	if err := saveSettings(a.db, s.sanitize()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
