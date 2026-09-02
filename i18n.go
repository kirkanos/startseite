// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"strings"
)

// i18n wird in die View-Daten eingebettet und stellt {{.Lang}} und {{.T "key"}}
// in den Templates bereit.
type i18n struct {
	Lang string
}

func (i i18n) T(key string) string { return tr(i.Lang, key) }

const langCookie = "lang"

// detectLang liest die Sprache aus dem Cookie, sonst aus Accept-Language, Standard "de".
func detectLang(r *http.Request) string {
	if c, err := r.Cookie(langCookie); err == nil && (c.Value == "de" || c.Value == "en") {
		return c.Value
	}
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Accept-Language")), "en") {
		return "en"
	}
	return "de"
}

// handleSetLang merkt sich die Sprache in einem langlebigen Cookie und leitet zurück.
func (a *App) handleSetLang(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code != "de" && code != "en" {
		code = "de"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     langCookie,
		Value:    code,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func tr(lang, key string) string {
	if m, ok := messages[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if v, ok := messages["de"][key]; ok {
		return v
	}
	return key
}

var messages = map[string]map[string]string{
	"de": {
		"search_placeholder":  "Seiten durchsuchen …",
		"add_link":            "Link hinzufügen",
		"log_out":             "Abmelden",
		"log_in":              "Anmelden",
		"all":                 "Alle",
		"manage_categories":   "Kategorien verwalten",
		"done":                "Fertig",
		"new_category":        "Neue Kategorie",
		"create":              "Anlegen",
		"save":                "Speichern",
		"saving":              "Speichern …",
		"color":               "Farbe",
		"delete_category":     "Kategorie löschen",
		"confirm_del_cat":     "Kategorie löschen? Die zugeordneten Links bleiben erhalten.",
		"no_links_yet":        "Noch keine Links. Füge deinen ersten Link hinzu.",
		"no_matches":          "Keine Treffer für diese Auswahl.",
		"uncategorized":       "Ohne Kategorie",
		"category_name_label": "Kategoriename",
		"edit":                "Bearbeiten",
		"refresh_preview":     "Vorschau neu erzeugen",
		"delete":              "Löschen",
		"public_badge":        "Öffentlich sichtbar (ohne Login)",
		"drag_links_here":     "Links hierher ziehen",
		"add_dialog_sub":      "URL einfügen — Titel & Screenshot-Vorschau werden automatisch geholt.",
		"url":                 "URL",
		"title_label":         "Titel",
		"title_optional":      "(optional — sonst automatisch)",
		"title_ph_auto":       "Wird aus der Seite ermittelt",
		"title_ph":            "Titel",
		"category":            "Kategorie",
		"category_none":       "Ohne Kategorie",
		"public_checkbox":     "Öffentlich sichtbar (ohne Login)",
		"cancel":              "Abbrechen",
		"creating_preview":    "Der Screenshot wird erstellt — das kann ein paar Sekunden dauern.",
		"generating_preview":  "Erzeuge Vorschau …",
		"edit_link":           "Link bearbeiten",
		"regenerate_preview":  "Screenshot-Vorschau neu erzeugen",
		"edit_saving_hint":    "Wird gespeichert — bei URL-Änderung oder neuer Vorschau kann das ein paar Sekunden dauern.",
		"login_sub":           "Passwort eingeben, um alle Links zu sehen und zu verwalten.",
		"password":            "Passwort",
		"stay_logged_in":      "Angemeldet bleiben (30 Tage)",
		"wrong_password":      "Falsches Passwort.",
		"no_public_links":     "Es sind noch keine öffentlichen Links vorhanden.",
		"edit_view":           "Ansicht bearbeiten",
		"view_options":        "Layout und Ansicht bearbeiten",
		"edit_view_hint":      "Kategorien am Griff ziehen zum Anordnen, an der rechten Kante ziehen für die Breite. Passen zwei Breiten zusammen in 12 Spalten, stehen die Kategorien nebeneinander.",
		"resize_category":     "Breite ziehen",
		"columns":             "Spalten",
		"cols_auto":           "Automatisch",
		"card_size":           "Kartenbreite",
		"card_size_hint":      "wirkt nur bei automatischen Spalten",
		"size_s":              "Schmal",
		"size_m":              "Mittel",
		"size_l":              "Breit",
		"show_thumbs":         "Screenshot-Vorschauen anzeigen",
		"view_saved_hint":     "Die Ansicht wird in der Datenbank gespeichert und gilt auf allen Geräten.",
		"drag_cats_hint":      "Am Griff ziehen, um die Reihenfolge der Kategorien zu ändern.",
		"reorder_category":    "Kategorie verschieben",
		"nsfw":                "NSFW",
		"nsfw_category":       "NSFW-Kategorie",
		"nsfw_short":          "18+",
		"show_nsfw":           "NSFW einblenden",
		"hide_nsfw":           "NSFW ausblenden",
		"nsfw_hint":           "NSFW-Kategorien sind im Dashboard standardmäßig ausgeblendet, ihre Vorschauen unscharf — und ohne Login nie sichtbar, auch wenn einzelne Links öffentlich markiert sind.",
	},
	"en": {
		"search_placeholder":  "Search sites …",
		"add_link":            "Add link",
		"log_out":             "Log out",
		"log_in":              "Log in",
		"all":                 "All",
		"manage_categories":   "Manage categories",
		"done":                "Done",
		"new_category":        "New category",
		"create":              "Add",
		"save":                "Save",
		"saving":              "Saving …",
		"color":               "Color",
		"delete_category":     "Delete category",
		"confirm_del_cat":     "Delete category? The assigned links will be kept.",
		"no_links_yet":        "No links yet. Add your first link.",
		"no_matches":          "No matches for this selection.",
		"uncategorized":       "Uncategorized",
		"category_name_label": "Category name",
		"edit":                "Edit",
		"refresh_preview":     "Regenerate preview",
		"delete":              "Delete",
		"public_badge":        "Publicly visible (no login)",
		"drag_links_here":     "Drag links here",
		"add_dialog_sub":      "Paste a URL — title & screenshot preview are fetched automatically.",
		"url":                 "URL",
		"title_label":         "Title",
		"title_optional":      "(optional — otherwise automatic)",
		"title_ph_auto":       "Determined from the page",
		"title_ph":            "Title",
		"category":            "Category",
		"category_none":       "Uncategorized",
		"public_checkbox":     "Publicly visible (no login)",
		"cancel":              "Cancel",
		"creating_preview":    "The screenshot is being created — this may take a few seconds.",
		"generating_preview":  "Creating preview …",
		"edit_link":           "Edit link",
		"regenerate_preview":  "Regenerate screenshot preview",
		"edit_saving_hint":    "Saving — if the URL changed or a new preview is requested this may take a few seconds.",
		"login_sub":           "Enter the password to see and manage all links.",
		"password":            "Password",
		"stay_logged_in":      "Stay logged in (30 days)",
		"wrong_password":      "Wrong password.",
		"no_public_links":     "There are no public links yet.",
		"edit_view":           "Edit view",
		"view_options":        "Edit layout and view",
		"edit_view_hint":      "Drag categories by the handle to arrange them, drag the right edge for the width. If two widths fit within 12 columns, the categories sit side by side.",
		"resize_category":     "Drag width",
		"columns":             "Columns",
		"cols_auto":           "Automatic",
		"card_size":           "Card width",
		"card_size_hint":      "only applies with automatic columns",
		"size_s":              "Narrow",
		"size_m":              "Medium",
		"size_l":              "Wide",
		"show_thumbs":         "Show screenshot previews",
		"view_saved_hint":     "The view is stored in the database and applies on all devices.",
		"drag_cats_hint":      "Drag the handle to change the order of the categories.",
		"reorder_category":    "Move category",
		"nsfw":                "NSFW",
		"nsfw_category":       "NSFW category",
		"nsfw_short":          "18+",
		"show_nsfw":           "Show NSFW",
		"hide_nsfw":           "Hide NSFW",
		"nsfw_hint":           "NSFW categories are hidden in the dashboard by default and their previews are blurred — and they are never visible without a login, even if individual links are marked public.",
	},
}
