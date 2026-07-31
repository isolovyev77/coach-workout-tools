// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
)

func TestSessionFromJarAcceptsCurrentCookieName(t *testing.T) {
	jar := newSessionJar(t,
		&http.Cookie{Name: btwbSessionCookieName, Value: "current-session"})

	if got := sessionFromJar(jar); got != "current-session" {
		t.Fatalf("sessionFromJar() = %q, want current-session", got)
	}
}

func TestSessionFromJarAcceptsLegacyCookieName(t *testing.T) {
	jar := newSessionJar(t,
		&http.Cookie{Name: btwbLegacySessionCookieName, Value: "legacy-session"})

	if got := sessionFromJar(jar); got != "legacy-session" {
		t.Fatalf("sessionFromJar() = %q, want legacy-session", got)
	}
}

func TestSessionFromJarPrefersCurrentCookieName(t *testing.T) {
	jar := newSessionJar(t,
		&http.Cookie{Name: btwbLegacySessionCookieName, Value: "legacy-session"},
		&http.Cookie{Name: btwbSessionCookieName, Value: "current-session"})

	if got := sessionFromJar(jar); got != "current-session" {
		t.Fatalf("sessionFromJar() = %q, want current-session", got)
	}
}

func newSessionJar(t *testing.T, cookies ...*http.Cookie) *cookiejar.Jar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New(): %v", err)
	}
	u, err := url.Parse("https://btwb.com/")
	if err != nil {
		t.Fatalf("url.Parse(): %v", err)
	}
	jar.SetCookies(u, cookies)
	return jar
}
