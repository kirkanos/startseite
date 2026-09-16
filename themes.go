// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Die Farbpaletten stehen in static/themes.css (erzeugt aus den Themes von
// Dashy, siehe tools/gen-themes.py). Damit die Liste nicht doppelt gepflegt
// werden muss, liest Go die Namen beim ersten Bedarf aus derselben Datei.

// baseThemes sind die eingebauten Designs, die keine Palette in themes.css haben.
var baseThemes = []string{"auto", "light", "dark"}

var (
	reThemeRule = regexp.MustCompile(`:root\[data-theme='([a-z0-9-]+)'\]`)
	themesOnce  sync.Once
	themeList   []string
	themeSet    map[string]bool
)

func loadThemes() {
	themesOnce.Do(func() {
		css, err := staticFS.ReadFile("static/themes.css")
		if err != nil {
			css = nil
		}
		var extra []string
		for _, m := range reThemeRule.FindAllStringSubmatch(string(css), -1) {
			if !slices.Contains(extra, m[1]) {
				extra = append(extra, m[1])
			}
		}
		slices.Sort(extra)
		themeList = append(append([]string{}, baseThemes...), extra...)
		themeSet = make(map[string]bool, len(themeList))
		for _, t := range themeList {
			themeSet[t] = true
		}
	})
}

// themeNames liefert alle wählbaren Designs: erst die eingebauten, dann die
// Paletten alphabetisch.
func themeNames() []string {
	loadThemes()
	return themeList
}

// validTheme prüft einen Design-Namen aus der Datenbank oder vom Browser.
func validTheme(name string) bool {
	loadThemes()
	return themeSet[name]
}

// themeLabel macht aus "nord-frost" ein lesbares "Nord Frost".
func themeLabel(name string) string {
	switch name {
	case "auto", "light", "dark":
		return "" // diese haben übersetzte Namen in i18n.go
	}
	parts := strings.Split(name, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// ThemeOption ist ein Eintrag der Design-Auswahl im Ansichts-Panel.
type ThemeOption struct {
	Name  string
	Label string
}

// paletteThemes liefert die übertragenen Dashy-Paletten (ohne auto/hell/dunkel,
// die eigene übersetzte Namen haben).
func paletteThemes() []ThemeOption {
	var out []ThemeOption
	for _, n := range themeNames() {
		if slices.Contains(baseThemes, n) {
			continue
		}
		out = append(out, ThemeOption{Name: n, Label: themeLabel(n)})
	}
	return out
}
