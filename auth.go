// Copyright 2026 kirkanos
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "session"

// checkPassword prüft das Passwort gegen den bcrypt-Hash aus der Config.
func (a *App) checkPassword(pw string) bool {
	if a.cfg.PasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(a.cfg.PasswordHash), []byte(pw)) == nil
}

// signSession erzeugt ein HMAC-signiertes, zustandsloses Session-Token.
func (a *App) signSession() string {
	payload := "authed." + strconv.FormatInt(time.Now().UnixNano(), 10)
	return payload + "." + a.sign(payload)
}

// verifySession prüft die HMAC-Signatur eines Tokens.
func (a *App) verifySession(token string) bool {
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return false
	}
	payload, sig := token[:i], token[i+1:]
	return hmac.Equal([]byte(sig), []byte(a.sign(payload)))
}

func (a *App) sign(payload string) string {
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// setSession setzt das Session-Cookie; remember => 30 Tage, sonst Session-Cookie.
func (a *App) setSession(w http.ResponseWriter, r *http.Request, remember bool) {
	c := &http.Cookie{
		Name:     sessionCookie,
		Value:    a.signSession(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
	}
	if remember {
		c.MaxAge = 60 * 60 * 24 * 30
	}
	http.SetCookie(w, c)
}

func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func (a *App) authed(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	return err == nil && a.verifySession(c.Value)
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
