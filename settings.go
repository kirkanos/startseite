// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// Settings sind die Ansichts-Einstellungen des Dashboards. Sie liegen in der
// Datenbank (Tabelle "settings") und nicht im Browser — damit dieselbe Ansicht
// auf jedem Rechner erscheint.
type Settings struct {
	Columns    string `json:"columns"`     // "auto" oder "2".."6"
	CardSize   string `json:"card_size"`   // "s" | "m" | "l" (Kartenbreite bei "auto")
	ShowThumbs bool   `json:"show_thumbs"` // Screenshot-Vorschauen anzeigen
}

func defaultSettings() Settings {
	return Settings{Columns: "auto", CardSize: "m", ShowThumbs: true}
}

// ThumbsAttr liefert den Wert für das data-thumbs-Attribut am <body>.
func (s Settings) ThumbsAttr() string {
	if s.ShowThumbs {
		return "on"
	}
	return "off"
}

var (
	validColumns = map[string]bool{"auto": true, "2": true, "3": true, "4": true, "5": true, "6": true}
	validSizes   = map[string]bool{"s": true, "m": true, "l": true}
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
	return s
}

// loadSettings liest die Einstellungen; fehlende Schlüssel bleiben auf Standard.
func loadSettings(db *sql.DB) Settings {
	s := defaultSettings()
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
		case "show_thumbs":
			s.ShowThumbs = v == "1"
		}
	}
	return s.sanitize()
}

func saveSettings(db *sql.DB, s Settings) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO settings (key, value) VALUES (?, ?)
	                         ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	pairs := [][2]string{
		{"columns", s.Columns},
		{"card_size", s.CardSize},
		{"show_thumbs", boolStr(s.ShowThumbs)},
	}
	for _, p := range pairs {
		if _, err := stmt.Exec(p[0], p[1]); err != nil {
			return err
		}
	}
	return tx.Commit()
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
