package main

import (
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

// ---- Views ----

type Group struct {
	ID    int64
	Name  string
	Color string
	Links []Link
}

type indexData struct {
	i18n
	Title      string
	Categories []Category
	Groups     []Group
	Total      int
	Counts     map[int64]int
}

// ---- Auth / Login ----

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.checkPassword(r.FormValue("password")) {
		// Fehler im Login-Modal der öffentlichen Startseite anzeigen.
		a.renderPublic(w, r, http.StatusUnauthorized, tr(detectLang(r), "wrong_password"))
		return
	}
	a.setSession(w, r, r.FormValue("remember") != "")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---- Dashboard ----

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	// Ohne Login zeigt die Einstiegsseite nur die öffentlichen Links.
	if !a.authed(r) {
		a.renderPublic(w, r, http.StatusOK, "")
		return
	}
	lang := detectLang(r)
	cats, err := listCategories(a.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	links, err := listLinks(a.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	byCat := map[int64][]Link{}
	counts := map[int64]int{}
	for _, l := range links {
		byCat[l.CategoryID] = append(byCat[l.CategoryID], l)
		counts[l.CategoryID]++
	}

	// Alle Kategorien als Sektionen rendern (auch leere) — sie sind Drop-Ziele
	// für Drag & Drop. "Ohne Kategorie" kommt zuletzt, sofern es Links gibt.
	var groups []Group
	if len(links) > 0 {
		for _, c := range cats {
			groups = append(groups, Group{ID: c.ID, Name: c.Name, Color: c.Color, Links: byCat[c.ID]})
		}
		groups = append(groups, Group{ID: 0, Name: tr(lang, "uncategorized"), Color: "var(--ink-faint)", Links: byCat[0]})
	}

	render(w, indexTmpl, http.StatusOK, indexData{
		i18n:       i18n{Lang: lang},
		Title:      "Startseite",
		Categories: cats,
		Groups:     groups,
		Total:      len(links),
		Counts:     counts,
	})
}

// ---- Öffentliche Seite (ohne Login) ----

type publicData struct {
	i18n
	Title      string
	Groups     []Group
	Total      int
	LoginError string // gesetzt, wenn ein Login-Versuch fehlschlug (öffnet das Modal)
}

// renderPublic rendert die öffentliche Einstiegsseite (nur öffentliche Links).
// loginError != "" öffnet das Login-Modal mit Fehlermeldung.
func (a *App) renderPublic(w http.ResponseWriter, r *http.Request, status int, loginError string) {
	lang := detectLang(r)
	cats, err := listCategories(a.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	links, err := listPublicLinks(a.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	byCat := map[int64][]Link{}
	for _, l := range links {
		byCat[l.CategoryID] = append(byCat[l.CategoryID], l)
	}
	// Nur Kategorien mit öffentlichen Links anzeigen.
	var groups []Group
	for _, c := range cats {
		if ls := byCat[c.ID]; len(ls) > 0 {
			groups = append(groups, Group{ID: c.ID, Name: c.Name, Color: c.Color, Links: ls})
		}
	}
	if ls := byCat[0]; len(ls) > 0 {
		groups = append(groups, Group{ID: 0, Name: tr(lang, "uncategorized"), Color: "var(--ink-faint)", Links: ls})
	}

	render(w, publicTmpl, status, publicData{
		i18n:       i18n{Lang: lang},
		Title:      "Startseite",
		Groups:     groups,
		Total:      len(links),
		LoginError: loginError,
	})
}

// ---- Links ----

func (a *App) handleAddLink(w http.ResponseWriter, r *http.Request) {
	rawURL, ok := normalizeURL(r.FormValue("url"))
	if !ok {
		http.Error(w, "Ungültige URL", http.StatusBadRequest)
		return
	}
	var catID any
	if id, err := strconv.ParseInt(r.FormValue("category_id"), 10, 64); err == nil && id > 0 {
		catID = id
	}

	meta := fetchMetadata(rawURL)
	title := r.FormValue("title")
	if title == "" {
		title = meta.Title
	}
	thumb := a.captureThumbnail(rawURL)

	_, err := a.db.Exec(
		`INSERT INTO links (url, title, description, favicon, thumbnail, category_id, public, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rawURL, title, nullify(meta.Description), nullify(meta.Favicon), nullify(thumb), catID,
		boolInt(r.FormValue("public") != ""), time.Now().Unix(),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Ungültige ID", http.StatusBadRequest)
		return
	}
	var thumb string
	_ = a.db.QueryRow(`SELECT COALESCE(thumbnail, '') FROM links WHERE id = ?`, id).Scan(&thumb)
	if _, err := a.db.Exec(`DELETE FROM links WHERE id = ?`, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.deleteThumbnail(thumb)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleRefreshLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Ungültige ID", http.StatusBadRequest)
		return
	}
	var linkURL, oldThumb string
	if err := a.db.QueryRow(`SELECT url, COALESCE(thumbnail, '') FROM links WHERE id = ?`, id).Scan(&linkURL, &oldThumb); err != nil {
		http.Error(w, "Link nicht gefunden", http.StatusNotFound)
		return
	}
	// Favicon frisch aus dem HTML ermitteln und Screenshot neu erzeugen.
	meta := fetchMetadata(linkURL)
	if thumb := a.captureThumbnail(linkURL); thumb != "" {
		if _, err := a.db.Exec(`UPDATE links SET thumbnail = ?, favicon = ? WHERE id = ?`, thumb, nullify(meta.Favicon), id); err == nil {
			a.deleteThumbnail(oldThumb)
		}
	} else {
		_, _ = a.db.Exec(`UPDATE links SET favicon = ? WHERE id = ?`, nullify(meta.Favicon), id)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---- Kategorien ----

func (a *App) handleAddCategory(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name fehlt", http.StatusBadRequest)
		return
	}
	color := r.FormValue("color")
	if color == "" {
		color = "#4f5bd5"
	}
	var maxOrder int
	_ = a.db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) FROM categories`).Scan(&maxOrder)
	// UNIQUE(name): doppelte Namen werden hier stillschweigend ignoriert.
	_, _ = a.db.Exec(`INSERT INTO categories (name, color, sort_order) VALUES (?, ?, ?)`, name, color, maxOrder+1)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Ungültige ID", http.StatusBadRequest)
		return
	}
	// Links bleiben erhalten (ON DELETE SET NULL).
	if _, err := a.db.Exec(`DELETE FROM categories WHERE id = ?`, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---- Thumbnails ----

func (a *App) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("file")) // schützt vor Path-Traversal

	// Ohne Login nur Thumbnails ausliefern, die zu einem öffentlichen Link gehören.
	if !a.authed(r) {
		var n int
		_ = a.db.QueryRow(`SELECT COUNT(*) FROM links WHERE thumbnail = ? AND public = 1`, name).Scan(&n)
		if n == 0 {
			http.NotFound(w, r)
			return
		}
	}

	path := filepath.Join(a.cfg.ThumbDir, name)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, path)
}

// ---- Helpers ----

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// nullify wandelt "" in nil, damit in SQLite NULL statt Leerstring landet.
func nullify(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
