// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// crossfitReplay stands in for CrossFit's sign-in and token endpoints, and
// records what the CLI actually sent.
type crossfitReplay struct {
	signinQuery url.Values
	signinBody  map[string]string
	// tokenGrant holds the exchange parameters however they were encoded, and
	// tokenTypes the Content-Type of each attempt in order - the exchange is
	// JSON-first with a form fallback, and tests pin both halves.
	tokenGrant map[string]string
	tokenTypes []string
	// rejectJSONExchange makes the token endpoint refuse a JSON body, the way a
	// server that only reads form encoding would.
	rejectJSONExchange bool
	// codeIn decides where the authorization code is returned from: "location"
	// or "body". The real API's shape is not contractual, so both are supported.
	codeIn string
}

func (r *crossfitReplay) start(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/users/v2/auth/signin", func(w http.ResponseWriter, req *http.Request) {
		r.signinQuery = req.URL.Query()
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &r.signinBody)

		if r.signinBody["password"] == "wrong" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"description":"Invalid payload","error":"CREDENTIALS_INVALID"}`))
			return
		}
		if r.codeIn == "body" {
			w.Write([]byte(`{"code":"auth-code-123"}`))
			return
		}
		w.Header().Set("Location",
			"https://affiliate.crossfit.com/tools/redirect?code=auth-code-123&state=x")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/users/v2/auth/token", func(w http.ResponseWriter, req *http.Request) {
		ct := req.Header.Get("Content-Type")
		r.tokenTypes = append(r.tokenTypes, ct)
		if strings.HasPrefix(ct, "application/json") {
			if r.rejectJSONExchange {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"invalid_request","error_description":"unsupported content type"}`))
				return
			}
			raw, _ := io.ReadAll(req.Body)
			grant := map[string]string{}
			_ = json.Unmarshal(raw, &grant)
			r.tokenGrant = grant
		} else {
			_ = req.ParseForm()
			grant := map[string]string{}
			for k := range req.PostForm {
				grant[k] = req.PostForm.Get(k)
			}
			r.tokenGrant = grant
		}
		w.Write([]byte(`{"access_token":"tok-abc","refresh_token":"ref-xyz","expires_in":3600}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func withAPI(t *testing.T, base string) {
	t.Helper()
	old := crossfitAPI
	crossfitAPI = base
	t.Cleanup(func() { crossfitAPI = old })
}

// The whole exchange, end to end: credentials in, tokens out.
func TestCrossFitSignInReturnsTokens(t *testing.T) {
	replay := &crossfitReplay{codeIn: "location"}
	withAPI(t, replay.start(t))

	token, refresh, expiry, _, err := crossfitSignIn("me@example.com", "correct", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-abc" || refresh != "ref-xyz" {
		t.Errorf("tokens = %q / %q", token, refresh)
	}
	if expiry.IsZero() || time.Until(expiry) < 50*time.Minute {
		t.Errorf("expiry = %v, want roughly an hour out", expiry)
	}
}

// Both of these were discovered by probing the live API and are easy to lose in
// a refactor: the OAuth parameters belong in the query string even though the
// credentials go in the body, and the body must carry a country and language.
func TestSignInSendsWhatTheAPIRequires(t *testing.T) {
	replay := &crossfitReplay{codeIn: "location"}
	withAPI(t, replay.start(t))

	if _, _, _, _, err := crossfitSignIn("me@example.com", "correct", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, key := range []string{"response_type", "code_challenge", "code_challenge_method",
		"client_id", "redirect_uri"} {
		if replay.signinQuery.Get(key) == "" {
			t.Errorf("query parameter %q was not sent; the API rejects the request without it", key)
		}
	}
	if replay.signinQuery.Get("code_challenge_method") != "S256" {
		t.Errorf("challenge method = %q, want S256", replay.signinQuery.Get("code_challenge_method"))
	}
	if replay.signinBody["country"] == "" || replay.signinBody["language"] == "" {
		t.Errorf("body = %v, want a country and language (the API demands both)", replay.signinBody)
	}
	if replay.signinBody["email"] != "me@example.com" {
		t.Errorf("email = %q", replay.signinBody["email"])
	}
}

// PKCE only protects anything if the verifier sent at the token step is the
// pre-image of the challenge sent at the sign-in step. A refactor that
// regenerated the pair between calls would still "work" against a lax server
// and silently drop the protection.
func TestSignInUsesAMatchingPKCEPair(t *testing.T) {
	replay := &crossfitReplay{codeIn: "location"}
	withAPI(t, replay.start(t))

	if _, _, _, _, err := crossfitSignIn("me@example.com", "correct", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	verifier := replay.tokenGrant["code_verifier"]
	if verifier == "" {
		t.Fatal("no code_verifier was sent to the token endpoint")
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if got := replay.signinQuery.Get("code_challenge"); got != want {
		t.Errorf("challenge %q is not the SHA-256 of the verifier that was sent", got)
	}
	if replay.tokenGrant["grant_type"] != "authorization_code" {
		t.Errorf("grant_type = %q", replay.tokenGrant["grant_type"])
	}
}

// A wrong password is the everyday case. It must read as an auth failure with a
// plain sentence, not as a server error with a JSON blob.
func TestSignInRejectsBadCredentialsCleanly(t *testing.T) {
	replay := &crossfitReplay{codeIn: "location"}
	withAPI(t, replay.start(t))

	_, _, _, _, err := crossfitSignIn("me@example.com", "wrong", 5*time.Second)
	if err == nil {
		t.Fatal("a wrong password was accepted")
	}
	if code := ExitCode(err); code != 4 {
		t.Errorf("exit code = %d, want 4 (auth)", code)
	}
	if strings.Contains(err.Error(), "{") {
		t.Errorf("error leaks raw JSON: %q", err)
	}
	if !strings.Contains(err.Error(), "email and password") {
		t.Errorf("error = %q, want it to name what was rejected", err)
	}
}

// The code may come back in a redirect or in the body; both were plausible when
// this was written, so both are handled.
func TestSignInReadsTheCodeFromEitherPlace(t *testing.T) {
	for _, where := range []string{"location", "body"} {
		replay := &crossfitReplay{codeIn: where}
		withAPI(t, replay.start(t))
		token, _, _, _, err := crossfitSignIn("me@example.com", "correct", 5*time.Second)
		if err != nil {
			t.Fatalf("code in %s: %v", where, err)
		}
		if token != "tok-abc" {
			t.Errorf("code in %s: token = %q", where, token)
		}
		if got := replay.tokenGrant["code"]; got != "auth-code-123" {
			t.Errorf("code in %s: exchanged %q", where, got)
		}
	}
}

// The redirect carries the authorization code. Following it would hand the code
// to the website instead of keeping it, and the sign-in would fail with no
// obvious cause.
func TestSignInDoesNotFollowTheRedirect(t *testing.T) {
	var redirectFollowed bool
	mux := http.NewServeMux()
	mux.HandleFunc("/users/v2/auth/signin", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Location", "/tools/redirect?code=auth-code-123")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/tools/redirect", func(w http.ResponseWriter, req *http.Request) {
		redirectFollowed = true
		w.Write([]byte("landing page"))
	})
	mux.HandleFunc("/users/v2/auth/token", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"access_token":"tok-abc","expires_in":3600}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	withAPI(t, srv.URL)

	if _, _, _, _, err := crossfitSignIn("me@example.com", "correct", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if redirectFollowed {
		t.Error("the client followed the redirect, handing the authorization code away")
	}
}
