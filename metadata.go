package main

import (
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Metadata struct {
	Title       string
	Description string
	Favicon     string
}

var (
	reOGTitle = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`)
	reTitle   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reDesc    = regexp.MustCompile(`(?i)<meta[^>]+name=["']description["'][^>]+content=["']([^"']*)["']`)
	reOGDesc  = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']*)["']`)
	reLinkTag = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reRelIcon = regexp.MustCompile(`(?i)\brel\s*=\s*["']?[^"'>]*icon`)
	reHref    = regexp.MustCompile(`(?i)\bhref\s*=\s*["']([^"']+)["']`)
)

// fetchMetadata holt Titel, Beschreibung und Favicon-URL (best effort).
func fetchMetadata(pageURL string) Metadata {
	host := ""
	if u, err := url.Parse(pageURL); err == nil {
		host = u.Hostname()
	}
	meta := Metadata{Title: host}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return meta
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StartseiteBot/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return meta
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // max 1 MB
	if err != nil {
		return meta
	}
	src := string(body)

	if m := reOGTitle.FindStringSubmatch(src); m != nil {
		meta.Title = clean(m[1])
	} else if m := reTitle.FindStringSubmatch(src); m != nil {
		meta.Title = clean(m[1])
	}
	if meta.Title == "" {
		meta.Title = host
	}

	if m := reDesc.FindStringSubmatch(src); m != nil {
		meta.Description = clean(m[1])
	} else if m := reOGDesc.FindStringSubmatch(src); m != nil {
		meta.Description = clean(m[1])
	}

	meta.Favicon = findFavicon(src, pageURL)
	return meta
}

// findFavicon liest das erste <link rel="...icon..."> aus dem HTML, löst relative
// URLs gegen die Seiten-URL auf und fällt sonst auf /favicon.ico zurück.
func findFavicon(html, pageURL string) string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	for _, tag := range reLinkTag.FindAllString(html, -1) {
		if !reRelIcon.MatchString(tag) {
			continue
		}
		m := reHref.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		if ref, err := url.Parse(strings.TrimSpace(m[1])); err == nil {
			return base.ResolveReference(ref).String()
		}
	}
	// Fallback: konventioneller Pfad.
	return base.Scheme + "://" + base.Host + "/favicon.ico"
}

func clean(s string) string {
	return strings.TrimSpace(html.UnescapeString(s))
}
