// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// handleEditLink ändert URL, Titel und Kategorie eines Links. Wenn sich die URL
// ändert oder "rescreenshot" gesetzt ist, wird die Vorschau neu erzeugt.
func (a *App) handleEditLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Ungültige ID", http.StatusBadRequest)
		return
	}

	var oldURL, oldTitle, oldThumb, oldFavicon string
	if err := a.db.QueryRow(
		`SELECT url, title, COALESCE(thumbnail, ''), COALESCE(favicon, '') FROM links WHERE id = ?`, id,
	).Scan(&oldURL, &oldTitle, &oldThumb, &oldFavicon); err != nil {
		http.Error(w, "Link nicht gefunden", http.StatusNotFound)
		return
	}

	newURL, ok := normalizeURL(r.FormValue("url"))
	if !ok {
		http.Error(w, "Ungültige URL", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	var catID any
	if v, err := strconv.ParseInt(r.FormValue("category_id"), 10, 64); err == nil && v > 0 {
		catID = v
	}
	urlChanged := newURL != oldURL
	rescreenshot := r.FormValue("rescreenshot") != ""

	favicon := oldFavicon
	if urlChanged {
		meta := fetchMetadata(newURL)
		favicon = meta.Favicon
		if title == "" {
			title = meta.Title
		}
	}
	if title == "" {
		title = oldTitle
	}

	thumb := oldThumb
	if urlChanged || rescreenshot {
		if nt := a.captureThumbnail(newURL); nt != "" {
			a.deleteThumbnail(oldThumb)
			thumb = nt
		}
	}

	if _, err := a.db.Exec(
		`UPDATE links SET url = ?, title = ?, favicon = ?, thumbnail = ?, category_id = ?, public = ? WHERE id = ?`,
		newURL, title, nullify(favicon), nullify(thumb), catID, boolInt(r.FormValue("public") != ""), id,
	); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleEditCategory ändert Name und Farbe einer Kategorie.
func (a *App) handleEditCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Ungültige ID", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name fehlt", http.StatusBadRequest)
		return
	}
	color := r.FormValue("color")
	if color == "" {
		color = "#4f5bd5"
	}
	// Bei doppeltem Namen (UNIQUE) schlägt das Update fehl — still ignorieren.
	_, _ = a.db.Exec(`UPDATE categories SET name = ?, color = ? WHERE id = ?`, name, color, id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleCategorySpan speichert die Breite einer Kategorie-Sektion im
// 12-Spalten-Raster der Übersicht. id 0 = Sektion "Ohne Kategorie".
func (a *App) handleCategorySpan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 0 {
		http.Error(w, "Ungültige ID", http.StatusBadRequest)
		return
	}
	span, err := strconv.Atoi(r.FormValue("span"))
	if err != nil {
		http.Error(w, "Ungültige Breite", http.StatusBadRequest)
		return
	}
	span = clampSpan(span)

	if id == 0 {
		err = setSetting(a.db, "uncat_span", strconv.Itoa(span))
	} else {
		_, err = a.db.Exec(`UPDATE categories SET layout_span = ? WHERE id = ?`, span, id)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type catOrderItem struct {
	ID        int64 `json:"id"`
	SortOrder int   `json:"sort_order"`
}

// handleReorderCategories speichert die per Drag & Drop gesetzte Reihenfolge der
// Kategorien. Erwartet ein JSON-Array im Body.
func (a *App) handleReorderCategories(w http.ResponseWriter, r *http.Request) {
	var items []catOrderItem
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&items); err != nil {
		http.Error(w, "Ungültige Daten", http.StatusBadRequest)
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE categories SET sort_order = ? WHERE id = ?`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	for _, it := range items {
		if _, err := stmt.Exec(it.SortOrder, it.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type reorderItem struct {
	ID         int64 `json:"id"`
	CategoryID int64 `json:"category_id"` // 0 = keine Kategorie
	SortOrder  int   `json:"sort_order"`
}

// handleReorder speichert die neue Reihenfolge und Kategorie-Zuordnung der Links
// (per Drag & Drop). Erwartet ein JSON-Array im Body.
func (a *App) handleReorder(w http.ResponseWriter, r *http.Request) {
	var items []reorderItem
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&items); err != nil {
		http.Error(w, "Ungültige Daten", http.StatusBadRequest)
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE links SET sort_order = ?, category_id = ? WHERE id = ?`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	for _, it := range items {
		var cat any
		if it.CategoryID > 0 {
			cat = it.CategoryID
		}
		if _, err := stmt.Exec(it.SortOrder, cat, it.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
