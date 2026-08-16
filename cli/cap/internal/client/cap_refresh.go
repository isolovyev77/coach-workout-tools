// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// CrossFit's access token lives a few hours. Two renewal strategies exist, and
// both live here.
//
// The obvious one - posting the refresh token back to the token endpoint - is
// what OAuth promises and what the toolkit frontend's code contains, but the
// server currently answers it with unauthorized_client for everyone, the
// frontend included (verified live, with the frontend's own freshly-issued
// token). What actually keeps the toolkit signed in is the identity session
// cookie: on a 401 the frontend bounces through its login route, where the
// cookie silently authorizes a new code, which is exchanged for fresh tokens.
//
// So the renewal here tries the refresh grant first - it is cheaper and would
// start working the day CrossFit turns it on - and falls back to the same
// silent reauthorization the site uses: GET /authorize with the stored session
// cookies and a fresh PKCE pair, read the code out of the redirect, exchange
// it. A coach signs in once; ordinary use renews the session on its own for as
// long as the identity session lives.

package client

import (
	"bytes"
	"cap-pp-cli/internal/config"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// TokenURL and AuthorizeURL are where CrossFit exchanges codes and authorizes
// sessions. Vars, not consts, so tests can point the renewal at a replay
// server.
var (
	TokenURL     = "https://c3po.crossfit.com/api/users/v2/auth/token"
	AuthorizeURL = "https://c3po.crossfit.com/api/users/v2/auth/authorize"
)

// Same public OAuth client the affiliate toolkit's frontend uses; tokens and
// codes are bound to it.
const (
	refreshClientID  = config.CrossFitAffiliateToolkitClientID
	renewRedirectURI = "https://affiliate.crossfit.com/tools/redirect"
	renewScope       = "user:full:read"
)

// canRenew reports whether any renewal material is stored at all.
func (c *Client) canRenew() bool {
	return c.Config != nil && (c.Config.RefreshToken != "" || len(c.Config.AuthCookies) > 0)
}

// refreshAccessToken renews the session and persists the fresh pair. The caller
// decides when: proactively when the stored expiry has passed, or reactively
// when the API answers 401.
func (c *Client) refreshAccessToken() error {
	if !c.canRenew() {
		return fmt.Errorf("the session has expired and nothing to renew it with is stored; run 'cap-pp-cli auth login'")
	}

	var grantErr error
	if c.Config.RefreshToken != "" {
		grantErr = c.renewByRefreshGrant()
		if grantErr == nil {
			return nil
		}
	}
	if len(c.Config.AuthCookies) > 0 {
		if err := c.renewBySilentAuthorize(); err == nil {
			return nil
		} else if grantErr == nil {
			grantErr = err
		}
	}
	if grantErr == nil {
		grantErr = fmt.Errorf("no renewal path succeeded")
	}
	return fmt.Errorf("CrossFit declined to renew the session (%v); run 'cap-pp-cli auth login'", grantErr)
}

// renewByRefreshGrant posts the stored refresh token back, the way the
// frontend's code says it should work.
func (c *Client) renewByRefreshGrant() error {
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.Config.RefreshToken,
		"client_id":     refreshClientID,
	})
	if err != nil {
		return fmt.Errorf("building the refresh request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, TokenURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building the refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cap-pp-cli")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh grant: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return c.saveTokenAnswer(resp.StatusCode, data, "refresh grant")
}

// renewBySilentAuthorize replays the frontend's real renewal: the identity
// cookies authorize a fresh code, and the code buys a fresh pair.
func (c *Client) renewBySilentAuthorize() error {
	verifier, challenge, err := newRenewPKCE()
	if err != nil {
		return err
	}
	q := url.Values{
		"response_type":         {"code"},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
		"client_id":             {refreshClientID},
		"redirect_uri":          {renewRedirectURI},
		"scope":                 {renewScope},
	}
	req, err := http.NewRequest(http.MethodGet, AuthorizeURL+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("building the authorize request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cap-pp-cli")
	req.Header.Set("Cookie", cookieHeader(c.Config.AuthCookies))

	// The code arrives in a redirect that must be read, not followed.
	transport := c.HTTPClient.Transport
	noFollow := &http.Client{Timeout: c.ConfiguredTimeout(), Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Do(req)
	if err != nil {
		return fmt.Errorf("silent authorize: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	// The identity session is a moving target: the server hands back a rotated
	// or extended cookie with the answer, and the browser keeping it is why a
	// browser session lives for weeks while a frozen copy dies in hours. Keep
	// whatever it hands back, on failure too - a bounce to the login page can
	// still carry a replacement.
	rotated := map[string]string{}
	for _, ck := range resp.Cookies() {
		if ck.Name != "" && ck.Value != "" {
			rotated[ck.Name] = ck.Value
		}
	}
	if len(rotated) > 0 {
		if err := c.Config.SaveAuthCookies(rotated); err != nil {
			return fmt.Errorf("keeping the rotated identity session: %w", err)
		}
	}

	location := resp.Header.Get("Location")
	code := codeFromRedirect(location)
	if code == "" {
		return fmt.Errorf("silent authorize: the identity session no longer signs codes (HTTP %d)", resp.StatusCode)
	}

	body, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": verifier,
		"client_id":     refreshClientID,
		"redirect_uri":  renewRedirectURI,
	})
	if err != nil {
		return fmt.Errorf("building the exchange request: %w", err)
	}
	exReq, err := http.NewRequest(http.MethodPost, TokenURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building the exchange request: %w", err)
	}
	exReq.Header.Set("Content-Type", "application/json")
	exReq.Header.Set("Accept", "application/json")
	exReq.Header.Set("User-Agent", "cap-pp-cli")

	exResp, err := c.HTTPClient.Do(exReq)
	if err != nil {
		return fmt.Errorf("exchanging the renewed code: %w", err)
	}
	defer exResp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(exResp.Body, 1<<20))
	return c.saveTokenAnswer(exResp.StatusCode, data, "silent authorize")
}

// saveTokenAnswer parses a token-endpoint answer and persists the pair.
func (c *Client) saveTokenAnswer(status int, data []byte, via string) error {
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	_ = json.Unmarshal(data, &payload)

	if status >= 400 || payload.AccessToken == "" {
		reason := payload.Description
		if reason == "" {
			reason = payload.Error
		}
		if reason == "" {
			reason = strings.TrimSpace(string(data))
			if len(reason) > 200 {
				reason = reason[:200]
			}
		}
		return fmt.Errorf("%s: %s", via, reason)
	}

	// Rotation is the server's choice: keep the old refresh token unless a new
	// one arrived, or a later renewal would have nothing to send.
	refresh := payload.RefreshToken
	if refresh == "" {
		refresh = c.Config.RefreshToken
	}
	var expiry time.Time
	if payload.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	if err := c.Config.SaveTokens(c.Config.ClientID, c.Config.ClientSecret,
		payload.AccessToken, refresh, expiry); err != nil {
		return fmt.Errorf("the session was renewed but could not be saved: %w", err)
	}
	return nil
}

func newRenewPKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generating the PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func codeFromRedirect(location string) string {
	if location == "" {
		return ""
	}
	u, err := url.Parse(location)
	if err != nil {
		return ""
	}
	return u.Query().Get("code")
}

// cookieHeader renders the stored cookies deterministically, so tests can
// assert on the exact header.
func cookieHeader(cookies map[string]string) string {
	keys := make([]string, 0, len(cookies))
	for k := range cookies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+cookies[k])
	}
	return strings.Join(parts, "; ")
}
