// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type App struct {
	cfg Config
	db  *sql.DB
}

func main() {
	// Unterkommando: bcrypt-Hash erzeugen (ersetzt ein separates Tool).
	//   startseite -hash "meinPasswort"
	if len(os.Args) == 3 && os.Args[1] == "-hash" {
		h, err := bcrypt.GenerateFromPassword([]byte(os.Args[2]), 12)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(h))
		return
	}

	cfg := loadConfig()
	if cfg.PasswordHash == "" {
		log.Println("WARNUNG: APP_PASSWORD_HASH ist leer — Login nicht möglich. Hash erzeugen mit: startseite -hash \"<passwort>\"")
	}
	if err := os.MkdirAll(cfg.ThumbDir, 0o755); err != nil {
		log.Fatalf("Datenverzeichnis: %v", err)
	}

	db, err := openDB(cfg.DataDir)
	if err != nil {
		log.Fatalf("Datenbank: %v", err)
	}
	defer db.Close()

	app := &App{cfg: cfg, db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", app.handleLogin)
	mux.HandleFunc("POST /logout", app.handleLogout)
	mux.HandleFunc("GET /lang/{code}", app.handleSetLang)
	mux.HandleFunc("GET /{$}", app.handleIndex)
	mux.HandleFunc("POST /links", app.handleAddLink)
	mux.HandleFunc("POST /links/reorder", app.handleReorder)
	mux.HandleFunc("POST /links/{id}/edit", app.handleEditLink)
	mux.HandleFunc("POST /links/{id}/delete", app.handleDeleteLink)
	mux.HandleFunc("POST /links/{id}/refresh", app.handleRefreshLink)
	mux.HandleFunc("POST /categories", app.handleAddCategory)
	mux.HandleFunc("POST /categories/reorder", app.handleReorderCategories)
	mux.HandleFunc("POST /categories/{id}/edit", app.handleEditCategory)
	mux.HandleFunc("POST /categories/{id}/delete", app.handleDeleteCategory)
	mux.HandleFunc("POST /settings", app.handleSaveSettings)
	mux.HandleFunc("GET /thumbnails/{file}", app.handleThumbnail)
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.withAuth(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("Startseite läuft auf http://0.0.0.0:%s (Daten: %s)", cfg.Port, cfg.DataDir)
	log.Fatal(srv.ListenAndServe())
}

// withAuth schützt nur schreibende Aktionen (POST). Lesende GET-Seiten sind
// öffentlich; die Handler entscheiden selbst, was sie ohne Login zeigen:
//   - "/" zeigt ohne Login die öffentlichen Links, mit Login das volle Dashboard
//   - "/thumbnails/…" liefert ohne Login nur Thumbnails öffentlicher Links
func (a *App) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.URL.Path != "/login" && !a.authed(r) {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
