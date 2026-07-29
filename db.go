package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite-Treiber, kein cgo
)

type Category struct {
	ID        int64
	Name      string
	Color     string
	SortOrder int
}

type Link struct {
	ID          int64
	URL         string
	Title       string
	Description string
	Favicon     string
	Thumbnail   string
	CategoryID  int64 // 0 = keine Kategorie
	Public      bool
	CreatedAt   int64
}

// openDB öffnet die SQLite-Datei und legt bei Bedarf das Schema an.
func openDB(dataDir string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)",
		filepath.Join(dataDir, "app.db"),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite verträgt nur einen Schreiber gleichzeitig.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	// Migration für bestehende Datenbanken: Spalte "public" nachrüsten.
	// Auf frischen DBs existiert sie bereits (aus dem CREATE) → Fehler ignorieren.
	_, _ = db.Exec(`ALTER TABLE links ADD COLUMN public INTEGER NOT NULL DEFAULT 0`)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		if _, err := db.Exec(
			`INSERT INTO categories (name, color, sort_order) VALUES (?, ?, ?)`,
			"Allgemein", "#4f5bd5", 0,
		); err != nil {
			return nil, err
		}
	}
	return db, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS categories (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL UNIQUE,
	color      TEXT NOT NULL DEFAULT '#4f5bd5',
	sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS links (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	url         TEXT NOT NULL,
	title       TEXT NOT NULL,
	description TEXT,
	favicon     TEXT,
	thumbnail   TEXT,
	category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
	sort_order  INTEGER NOT NULL DEFAULT 0,
	public      INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL
);
`

func listCategories(db *sql.DB) ([]Category, error) {
	rows, err := db.Query(`SELECT id, name, color, sort_order FROM categories ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func listLinks(db *sql.DB) ([]Link, error) {
	return queryLinks(db, "")
}

// listPublicLinks liefert nur die als öffentlich markierten Links.
func listPublicLinks(db *sql.DB) ([]Link, error) {
	return queryLinks(db, "WHERE public = 1")
}

func queryLinks(db *sql.DB, where string) ([]Link, error) {
	rows, err := db.Query(`
		SELECT id, url, title,
		       COALESCE(description, ''), COALESCE(favicon, ''),
		       COALESCE(thumbnail, ''), COALESCE(category_id, 0), public, created_at
		FROM links ` + where + `
		ORDER BY sort_order, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Link
	for rows.Next() {
		var l Link
		var pub int
		if err := rows.Scan(&l.ID, &l.URL, &l.Title, &l.Description, &l.Favicon,
			&l.Thumbnail, &l.CategoryID, &pub, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.Public = pub != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

// normalizeURL ergänzt ein fehlendes Schema und validiert die URL.
func normalizeURL(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	if !hasScheme(raw) {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

func hasScheme(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ':' {
			return i+2 < len(s) && s[i+1] == '/' && s[i+2] == '/'
		}
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return false
}
