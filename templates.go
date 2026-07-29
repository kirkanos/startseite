// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

//go:embed templates/*.html
var tmplFS embed.FS

//go:embed static/*
var staticFS embed.FS

var funcs = template.FuncMap{
	"host":     hostOf,
	"initials": initials,
	"lower":    strings.ToLower,
}

var (
	indexTmpl  = template.Must(template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/layout.html", "templates/index.html"))
	publicTmpl = template.Must(template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/layout.html", "templates/public.html"))
)

func render(w http.ResponseWriter, t *template.Template, status int, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

func initials(title string) string {
	var b []rune
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b = append(b, unicode.ToUpper(r))
			if len(b) == 2 {
				break
			}
		}
	}
	if len(b) == 0 {
		return "•"
	}
	return string(b)
}
