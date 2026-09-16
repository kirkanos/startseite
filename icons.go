// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha1"
	"database/sql"
	_ "embed" // für //go:embed der Emoji-Liste
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Symbole für Links und Kategorien folgen den Kennungen von Dashy:
//
//	si-jellyfin      simple-icons (Markenlogos)
//	mdi-home         Material Design Icons (einfarbig)
//	sh-jellyfin      selfh.st (Selfhosted-Logos)
//	hl-jellyfin      dashboard-icons von Homarr Labs
//	fas fa-rocket    Font Awesome (einfarbig); auch "fa-solid-rocket"
//	🚀               Emoji, direkt als Text
//	https://…/x.png  beliebige Bild-URL
//	favicon          das automatisch geholte Favicon der Seite (Standard)
//
// Alles außer Emoji wird einmal vom CDN geholt und unter data/icons/ abgelegt.
// Ausgeliefert wird danach nur noch lokal: das Dashboard kommt ohne fremde
// Requests aus und funktioniert auch ohne Internet.

// Zeichenart eines Symbols — entscheidet, wie das Template es einbaut.
const (
	iconImg   = "img"   // farbige Grafik
	iconMask  = "mask"  // einfarbige Silhouette, nimmt die Textfarbe an
	iconEmoji = "emoji" // direkt als Text
)

// iconSpecMax begrenzt die Länge einer Kennung (Schutz vor Unfug in der DB).
const iconSpecMax = 300

const (
	cdnSimpleIcons = "https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/%s.svg"
	cdnMDI         = "https://cdn.jsdelivr.net/npm/@mdi/svg@latest/svg/%s.svg"
	cdnSelfhst     = "https://cdn.jsdelivr.net/gh/selfhst/icons@main/svg/%s.svg"
	cdnHomarr      = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons@main/svg/%s.svg"
	cdnFontAwesome = "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6.7.2/svgs/%s/%s.svg"
)

var (
	reIconName  = regexp.MustCompile(`^[a-z0-9]+(?:[-.][a-z0-9]+)*$`)
	reSVGOpen   = regexp.MustCompile(`(?is)^\s*(?:<\?xml[^>]*\?>\s*)?(?:<!--.*?-->\s*)*<svg\b`)
	reSVGFill   = regexp.MustCompile(`(?i)\sfill="(?:#[0-9a-f]{3,8}|black|currentColor)"`)
	iconClient  = &http.Client{Timeout: 20 * time.Second}
	iconFetchMu sync.Mutex // ein Download zur Zeit: die CDNs mögen keine Salven
)

// faStyles bildet die Font-Awesome-Präfixe auf die Ordner der freien Variante ab.
var faStyles = map[string]string{
	"fas": "solid", "far": "regular", "fab": "brands",
	"solid": "solid", "regular": "regular", "brands": "brands",
}

// icon beschreibt ein aufgelöstes Symbol, so wie das Template es braucht.
type icon struct {
	Kind string // iconImg | iconMask | iconEmoji | "" (keins)
	Src  string // Pfad unter /icons/… bei img und mask
	Text string // das Zeichen bei emoji
}

// Empty sagt dem Template, ob es auf die Initialen zurückfallen muss.
func (i icon) Empty() bool { return i.Kind == "" }

// MaskStyle liefert den Inline-Style für einfarbige Symbole. Src ist ein von
// uns erzeugter Dateiname (Hash + Endung), daher unbedenklich als template.CSS.
func (i icon) MaskStyle() template.CSS {
	return template.CSS(`--ic:url("` + i.Src + `")`)
}

// remoteIcon ist das Ergebnis der Kennungs-Auflösung, bevor etwas geladen wurde.
type remoteIcon struct {
	kind string
	url  string
	ext  string
	slug string // bei simple-icons für die Markenfarbe
}

// isEmojiSpec erkennt eine Kennung, die schlicht aus Emoji besteht.
func isEmojiSpec(s string) bool {
	if s == "" || len([]rune(s)) > 3 {
		return false
	}
	for _, r := range s {
		if r < 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// resolveIconSpec übersetzt eine Kennung in Herkunfts-URL und Zeichenart.
// ok=false heißt: keine brauchbare Kennung (dann gilt das Favicon).
func resolveIconSpec(spec string) (remoteIcon, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" || len(spec) > iconSpecMax || strings.EqualFold(spec, "favicon") {
		return remoteIcon{}, false
	}
	if isEmojiSpec(spec) {
		return remoteIcon{kind: iconEmoji}, true
	}
	if strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "http://") {
		u, err := url.Parse(spec)
		if err != nil || u.Host == "" {
			return remoteIcon{}, false
		}
		return remoteIcon{kind: iconImg, url: spec, ext: extOf(u.Path)}, true
	}

	// "fas fa-rocket" / "fab fa-github" — die Schreibweise aus Font Awesome.
	if f := strings.Fields(spec); len(f) == 2 && strings.HasPrefix(f[1], "fa-") {
		if style, ok := faStyles[strings.ToLower(f[0])]; ok {
			name := strings.TrimPrefix(f[1], "fa-")
			if reIconName.MatchString(name) {
				return remoteIcon{kind: iconMask, url: fmt.Sprintf(cdnFontAwesome, style, name), ext: ".svg"}, true
			}
		}
		return remoteIcon{}, false
	}

	prefix, name, found := strings.Cut(strings.ToLower(spec), "-")
	if !found || !reIconName.MatchString(name) {
		return remoteIcon{}, false
	}
	switch prefix {
	case "si":
		return remoteIcon{kind: iconImg, url: fmt.Sprintf(cdnSimpleIcons, name), ext: ".svg", slug: name}, true
	case "mdi":
		return remoteIcon{kind: iconMask, url: fmt.Sprintf(cdnMDI, name), ext: ".svg"}, true
	case "sh":
		return remoteIcon{kind: iconImg, url: fmt.Sprintf(cdnSelfhst, name), ext: ".svg"}, true
	case "hl":
		return remoteIcon{kind: iconImg, url: fmt.Sprintf(cdnHomarr, name), ext: ".svg"}, true
	case "fa":
		// "fa-solid-rocket": Stil und Name stecken beide hinter dem Präfix.
		style, rest, ok := strings.Cut(name, "-")
		if folder, known := faStyles[style]; ok && known && reIconName.MatchString(rest) {
			return remoteIcon{kind: iconMask, url: fmt.Sprintf(cdnFontAwesome, folder, rest), ext: ".svg"}, true
		}
	}
	return remoteIcon{}, false
}

func extOf(path string) string {
	switch e := strings.ToLower(filepath.Ext(path)); e {
	case ".svg", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".ico":
		return e
	default:
		return ".png"
	}
}

// iconKey ist der Dateiname im Cache: ein kurzer Hash der Kennung.
func iconKey(spec, ext string) string {
	sum := sha1.Sum([]byte(spec))
	return hex.EncodeToString(sum[:])[:16] + ext
}

// ---- Lokaler Cache ----

// iconDir ist das Verzeichnis für geholte Symbole (neben den Thumbnails).
func (a *App) iconDir() string { return filepath.Join(a.cfg.DataDir, "icons") }

// lookupIcon liefert ein Symbol aus dem Cache. Bewusst ohne Netz: das Rendern
// einer Seite darf nie auf ein CDN warten. Fehlt das Symbol noch, kommt ein
// leeres icon zurück und die Seite fällt auf Favicon bzw. Initialen zurück;
// geholt wird es beim Speichern (warmIcon) oder beim Start (warmIcons).
func (a *App) lookupIcon(spec string) icon {
	r, ok := resolveIconSpec(spec)
	if !ok {
		return icon{}
	}
	if r.kind == iconEmoji {
		return icon{Kind: iconEmoji, Text: strings.TrimSpace(spec)}
	}

	var kind, file string
	switch err := a.db.QueryRow(`SELECT kind, file FROM icons WHERE spec = ?`, spec).Scan(&kind, &file); err {
	case nil:
		if _, statErr := os.Stat(filepath.Join(a.iconDir(), file)); statErr == nil {
			return icon{Kind: kind, Src: "/icons/" + file}
		}
		// Datei weg (etwa: DB gesichert, data/ nicht) → beim nächsten Warmlauf neu.
	case sql.ErrNoRows:
	default:
		log.Printf("Symbol %q: %v", spec, err)
	}
	return icon{}
}

// warmIcon stellt sicher, dass ein Symbol im Cache liegt, und holt es sonst.
// Wird beim Speichern eines Links oder einer Kategorie aufgerufen, damit die
// Auswahl sofort sichtbar ist.
func (a *App) warmIcon(spec string) {
	r, ok := resolveIconSpec(spec)
	if !ok || r.kind == iconEmoji {
		return
	}
	if !a.lookupIcon(spec).Empty() {
		return
	}
	if _, err := a.fetchIcon(spec, r); err != nil {
		log.Printf("Symbol %q konnte nicht geholt werden: %v", spec, err)
	}
}

// warmIcons holt beim Start alle Symbole nach, die noch nicht im Cache liegen —
// etwa nach dem Umzug auf einen anderen Rechner oder einer wiederhergestellten
// Datenbank. Läuft im Hintergrund, der Dienst startet ohne darauf zu warten.
func (a *App) warmIcons() {
	rows, err := a.db.Query(
		`SELECT icon FROM links WHERE icon <> ''
		 UNION SELECT icon FROM categories WHERE icon <> ''`)
	if err != nil {
		return
	}
	defer rows.Close()
	var specs []string
	for rows.Next() {
		var spec string
		if err := rows.Scan(&spec); err == nil {
			specs = append(specs, spec)
		}
	}
	for _, spec := range specs {
		a.warmIcon(spec)
	}
}

// fetchIcon lädt das Symbol vom CDN, legt es ab und merkt es sich in der DB.
func (a *App) fetchIcon(spec string, r remoteIcon) (icon, error) {
	iconFetchMu.Lock()
	defer iconFetchMu.Unlock()

	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return icon{}, err
	}
	req.Header.Set("User-Agent", "StartseiteBot/1.0")
	resp, err := iconClient.Do(req)
	if err != nil {
		return icon{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return icon{}, fmt.Errorf("Symbol %q: HTTP %d", spec, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10)) // max 512 KB
	if err != nil {
		return icon{}, err
	}

	isSVG := reSVGOpen.Match(body)
	if r.ext == ".svg" && !isSVG {
		// Die CDNs antworten auf unbekannte Namen gern mit einer HTML-Seite.
		return icon{}, fmt.Errorf("Symbol %q: kein SVG geliefert", spec)
	}
	kind, ext := r.kind, r.ext
	if isSVG {
		ext = ".svg"
		if r.slug != "" {
			// simple-icons liefert schwarze Pfade; mit der Markenfarbe aus dem
			// Index wird daraus ein farbiges Logo, sonst eine Silhouette, die
			// sich die Textfarbe nimmt.
			if hexColor := a.simpleIconColor(r.slug); hexColor != "" {
				body = colorizeSVG(body, "#"+hexColor)
			} else {
				kind = iconMask
			}
		}
	}

	if err := os.MkdirAll(a.iconDir(), 0o755); err != nil {
		return icon{}, err
	}
	file := iconKey(spec, ext)
	if err := os.WriteFile(filepath.Join(a.iconDir(), file), body, 0o644); err != nil {
		return icon{}, err
	}
	if _, err := a.db.Exec(
		`INSERT INTO icons (spec, kind, file, fetched_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(spec) DO UPDATE SET kind = excluded.kind, file = excluded.file, fetched_at = excluded.fetched_at`,
		spec, kind, file, time.Now().Unix(),
	); err != nil {
		return icon{}, err
	}
	return icon{Kind: kind, Src: "/icons/" + file}, nil
}

// colorizeSVG setzt eine Füllfarbe auf dem <svg>-Element. Vorhandene Angaben
// auf dem Wurzelelement werden ersetzt; farbige Mehrpfad-Logos bleiben heil,
// weil deren Pfade ihre eigene fill-Angabe tragen.
func colorizeSVG(body []byte, color string) []byte {
	loc := reSVGOpen.FindIndex(body)
	if loc == nil {
		return body
	}
	end := loc[1]
	head := reSVGFill.ReplaceAll(body[:end], nil)
	return append(append(append([]byte{}, head...), []byte(` fill="`+color+`"`)...), body[end:]...)
}

// ---- Suchindex für den Symbol-Wähler ----

// iconHit ist ein Treffer für den Wähler im Browser.
type iconHit struct {
	ID    string `json:"id"`            // die Kennung, die gespeichert wird
	Label string `json:"label"`         // lesbarer Name
	Set   string `json:"set"`           // si | mdi | sh | hl | fa | emoji
	URL   string `json:"url,omitempty"` // Vorschau, siehe withPreviewURLs
}

// withPreviewURLs hängt an jeden Treffer die CDN-Adresse für die Vorschau.
//
// Der Wähler zeigt bis zu 120 Symbole auf einmal; die alle einzeln durch den
// Server zu schleusen wäre langsam und für die CDNs unhöflich. Die Vorschau im
// Wähler lädt deshalb direkt von der Quelle — das betrifft nur den angemeldeten
// Benutzer im Auswahldialog. Das Dashboard selbst bleibt frei von fremden
// Adressen: gespeichert und lokal abgelegt wird erst das gewählte Symbol.
func withPreviewURLs(hits []iconHit) []iconHit {
	for i, h := range hits {
		if h.Set == "emoji" {
			continue
		}
		if r, ok := resolveIconSpec(h.ID); ok {
			hits[i].URL = r.url
		}
	}
	return hits
}

// iconSet beschreibt eine Symbolsammlung und wie ihr Index gelesen wird.
type iconSet struct {
	name  string
	url   string
	parse func([]byte) ([]iconHit, error)
}

//go:embed assets/emoji.json
var emojiJSON []byte

var iconSets = []iconSet{
	{"si", "https://cdn.jsdelivr.net/npm/simple-icons@latest/data/simple-icons.json", parseSimpleIcons},
	{"mdi", "https://cdn.jsdelivr.net/npm/@mdi/svg@latest/meta.json", parseMDI},
	{"sh", "https://cdn.jsdelivr.net/gh/selfhst/icons@main/index.json", parseSelfhst},
	{"hl", "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons@main/tree.json", parseHomarr},
	{"fa", "https://data.jsdelivr.com/v1/packages/npm/@fortawesome/fontawesome-free@6.7.2?structure=flat", parseFontAwesome},
}

func parseSimpleIcons(b []byte) ([]iconHit, error) {
	var raw []struct{ Title, Slug, Hex string }
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]iconHit, 0, len(raw))
	for _, e := range raw {
		if e.Slug == "" {
			continue
		}
		out = append(out, iconHit{ID: "si-" + e.Slug, Label: e.Title, Set: "si"})
	}
	return out, nil
}

func parseMDI(b []byte) ([]iconHit, error) {
	var raw []struct {
		Name    string   `json:"name"`
		Aliases []string `json:"aliases"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]iconHit, 0, len(raw))
	for _, e := range raw {
		label := strings.ReplaceAll(e.Name, "-", " ")
		if len(e.Aliases) > 0 {
			label += " · " + strings.Join(e.Aliases, " ")
		}
		out = append(out, iconHit{ID: "mdi-" + e.Name, Label: label, Set: "mdi"})
	}
	return out, nil
}

func parseSelfhst(b []byte) ([]iconHit, error) {
	var raw []struct {
		Name      string `json:"Name"`
		Reference string `json:"Reference"`
		SVG       string `json:"SVG"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]iconHit, 0, len(raw))
	for _, e := range raw {
		if e.Reference == "" || !strings.EqualFold(e.SVG, "yes") {
			continue
		}
		out = append(out, iconHit{ID: "sh-" + e.Reference, Label: e.Name, Set: "sh"})
	}
	return out, nil
}

func parseHomarr(b []byte) ([]iconHit, error) {
	var raw struct {
		SVG []string `json:"svg"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]iconHit, 0, len(raw.SVG))
	for _, f := range raw.SVG {
		name := strings.TrimSuffix(f, ".svg")
		out = append(out, iconHit{ID: "hl-" + name, Label: strings.ReplaceAll(name, "-", " "), Set: "hl"})
	}
	return out, nil
}

var reFAPath = regexp.MustCompile(`^/svgs/(solid|regular|brands)/([^/]+)\.svg$`)

func parseFontAwesome(b []byte) ([]iconHit, error) {
	var raw struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	var out []iconHit
	for _, f := range raw.Files {
		m := reFAPath.FindStringSubmatch(f.Name)
		if m == nil {
			continue
		}
		out = append(out, iconHit{
			ID:    "fa-" + m[1] + "-" + m[2],
			Label: strings.ReplaceAll(m[2], "-", " ") + " · " + m[1],
			Set:   "fa",
		})
	}
	return out, nil
}

func parseEmoji(b []byte) ([]iconHit, error) {
	var raw []struct{ C, N string }
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]iconHit, 0, len(raw))
	for _, e := range raw {
		out = append(out, iconHit{ID: e.C, Label: e.N, Set: "emoji"})
	}
	return out, nil
}

// indexMaxAge legt fest, wie lange ein geholter Index gilt, bevor er vom CDN
// aufgefrischt wird.
const indexMaxAge = 7 * 24 * time.Hour

type iconIndex struct {
	mu      sync.Mutex
	entries map[string][]iconHit // Sammlung → Einträge
	colors  map[string]string    // simple-icons: Slug → Hex-Farbe
}

// entriesFor liefert den Index einer Sammlung, lädt ihn bei Bedarf.
func (a *App) entriesFor(set string) []iconHit {
	a.icons.mu.Lock()
	defer a.icons.mu.Unlock()
	if a.icons.entries == nil {
		a.icons.entries = map[string][]iconHit{}
	}
	if e, ok := a.icons.entries[set]; ok {
		return e
	}
	if set == "emoji" {
		e, err := parseEmoji(emojiJSON)
		if err != nil {
			e = nil
		}
		a.icons.entries[set] = e
		return e
	}
	for _, s := range iconSets {
		if s.name != set {
			continue
		}
		body, err := a.indexBody(s)
		if err != nil {
			return nil // nicht merken: beim nächsten Mal neu versuchen
		}
		e, err := s.parse(body)
		if err != nil {
			return nil
		}
		a.icons.entries[set] = e
		return e
	}
	return nil
}

// indexBody liefert den rohen Index einer Sammlung aus data/icons/index/ und
// holt ihn vom CDN, wenn er fehlt oder zu alt ist.
func (a *App) indexBody(s iconSet) ([]byte, error) {
	dir := filepath.Join(a.iconDir(), "index")
	path := filepath.Join(dir, s.name+".json")
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) < indexMaxAge {
		if b, err := os.ReadFile(path); err == nil {
			return b, nil
		}
	}
	req, err := http.NewRequest(http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "StartseiteBot/1.0")
	resp, err := iconClient.Do(req)
	if err != nil {
		// Kein Netz: ein alter Index ist besser als gar keiner.
		if b, readErr := os.ReadFile(path); readErr == nil {
			return b, nil
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if b, readErr := os.ReadFile(path); readErr == nil {
			return b, nil
		}
		return nil, fmt.Errorf("Index %s: HTTP %d", s.name, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // max 8 MB
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err == nil {
		_ = os.WriteFile(path, body, 0o644)
	}
	return body, nil
}

// simpleIconColor liefert die Markenfarbe eines simple-icons-Eintrags.
func (a *App) simpleIconColor(slug string) string {
	a.icons.mu.Lock()
	if a.icons.colors != nil {
		c := a.icons.colors[slug]
		a.icons.mu.Unlock()
		return c
	}
	a.icons.mu.Unlock()

	body, err := a.indexBody(iconSets[0]) // simple-icons
	if err != nil {
		return ""
	}
	var raw []struct{ Slug, Hex string }
	if err := json.Unmarshal(body, &raw); err != nil {
		return ""
	}
	colors := make(map[string]string, len(raw))
	for _, e := range raw {
		colors[e.Slug] = e.Hex
	}
	a.icons.mu.Lock()
	a.icons.colors = colors
	a.icons.mu.Unlock()
	return colors[slug]
}

// searchIcons durchsucht eine Sammlung. Ein leeres q liefert den Anfang der
// Liste, damit der Wähler beim Öffnen nicht leer ist.
func (a *App) searchIcons(set, q string, limit int) []iconHit {
	entries := a.entriesFor(set)
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]iconHit, 0, limit)
	// Zwei Durchgänge: erst Treffer am Wortanfang, dann der Rest.
	for _, exact := range []bool{true, false} {
		for _, e := range entries {
			if len(out) >= limit {
				return out
			}
			if q == "" && !exact {
				continue
			}
			id := strings.ToLower(e.ID)
			label := strings.ToLower(e.Label)
			var hit bool
			switch {
			case q == "":
				hit = exact
			case exact:
				hit = strings.HasPrefix(label, q) || strings.HasPrefix(trimSetPrefix(id), q)
			default:
				hit = strings.Contains(label, q) || strings.Contains(id, q)
			}
			if hit && !containsHit(out, e.ID) {
				out = append(out, e)
			}
		}
	}
	return out
}

func trimSetPrefix(id string) string {
	if _, rest, ok := strings.Cut(id, "-"); ok {
		return rest
	}
	return id
}

func containsHit(hits []iconHit, id string) bool {
	for _, h := range hits {
		if h.ID == id {
			return true
		}
	}
	return false
}

// ---- HTTP ----

// cleanIconSpec normalisiert eine Kennung aus einem Formular. Was sich nicht
// auflösen lässt, wird verworfen — dann gilt wieder das Favicon.
func cleanIconSpec(raw string) string {
	spec := strings.TrimSpace(raw)
	if spec == "" || strings.EqualFold(spec, "favicon") {
		return ""
	}
	if _, ok := resolveIconSpec(spec); !ok {
		return ""
	}
	return spec
}

// handleIconFile liefert ein geholtes Symbol aus data/icons/. Symbole sind
// keine Nutzerdaten (nur Logos aus öffentlichen Sammlungen) und werden daher
// auch ohne Login ausgeliefert — die öffentliche Seite braucht sie.
func (a *App) handleIconFile(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("file")) // schützt vor Path-Traversal
	if name == "." || name == "/" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(a.iconDir(), name)
	// Der Dateiname ist der Hash der Kennung: ändert sich das Symbol, ändert
	// sich der Name. Deshalb darf lange gecacht werden.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if strings.HasSuffix(name, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	http.ServeFile(w, r, path)
}

// handleIconSearch beantwortet die Suche des Symbol-Wählers. Nur mit Login:
// GET ist in dieser Anwendung sonst offen, und der Wähler gehört zur Verwaltung.
func (a *App) handleIconSearch(w http.ResponseWriter, r *http.Request) {
	if !a.authed(r) {
		http.Error(w, "Nicht angemeldet", http.StatusForbidden)
		return
	}
	set := r.URL.Query().Get("set")
	known := set == "emoji"
	for _, s := range iconSets {
		if s.name == set {
			known = true
		}
	}
	if !known {
		http.Error(w, "Unbekannte Sammlung", http.StatusBadRequest)
		return
	}
	hits := withPreviewURLs(a.searchIcons(set, r.URL.Query().Get("q"), 120))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(hits)
}

// handleIconPreview liefert die Vorschau eines noch nicht gewählten Symbols.
// Der Wähler zeigt damit Treffer an, ohne dass sie schon gespeichert sind.
//
// Nur mit Login: der Aufruf lässt den Server eine fremde Adresse abrufen (bei
// einer selbst eingetippten Bild-URL sogar eine frei gewählte). Ohne diese
// Sperre wäre das ein offener Weg, den Server Adressen in seinem eigenen Netz
// abrufen zu lassen.
func (a *App) handleIconPreview(w http.ResponseWriter, r *http.Request) {
	if !a.authed(r) {
		http.Error(w, "Nicht angemeldet", http.StatusForbidden)
		return
	}
	spec := cleanIconSpec(r.URL.Query().Get("spec"))
	if spec == "" {
		http.NotFound(w, r)
		return
	}
	ic := a.lookupIcon(spec)
	if ic.Empty() {
		a.warmIcon(spec)
		ic = a.lookupIcon(spec)
	}
	if ic.Src == "" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, ic.Src, http.StatusFound)
}
