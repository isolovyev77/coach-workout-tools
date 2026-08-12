// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Refreshing the Trenda session when the server says the cookies
// have expired.
//
// Trenda hands out short-lived cookies and renews them at
// /api/v1/coach/refresh-token: post the current cookies, get new ones back in
// Set-Cookie. The Node helper already did this; without the same behaviour here
// the direct CLI would work right after signing in and then start failing with
// 401 an hour later, which is the confusing half of the bug this fixes.
//
// The refreshed cookies are written back to the same store the sign-in helper
// uses, so both halves stay on one session.

package client

import (
	"fmt"
	"io"
	"os"
	"net/http"
	"strings"

	"trenda-pp-cli/internal/config"
)

// trendaRefreshPath is the endpoint that renews the session cookies.
const trendaRefreshPath = "/api/v1/coach/refresh-token"

// refreshTrendaSession exchanges the stored cookies for fresh ones and returns
// the new Cookie header. It reports ok=false when there is nothing to refresh
// (no store, or the server declined), leaving the caller to surface the
// original 401 rather than a confusing secondary failure.
func (c *Client) refreshTrendaSession() (header string, ok bool) {
	// An explicitly supplied session is the caller's to manage: refreshing it
	// would write a credential they did not ask us to persist.
	if c.Config == nil || c.Config.AuthSource != "login" {
		return "", false
	}
	path := config.TrendaStorePath()
	store, err := config.LoadTrendaStore(path)
	if err != nil || store == nil {
		return "", false
	}
	current := store.CookieHeader()
	if current == "" {
		return "", false
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+trendaRefreshPath, strings.NewReader("{}"))
	if err != nil {
		return "", false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", current)
	if c.Config != nil {
		for k, v := range c.Config.Headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}
	if !store.MergeSetCookie(resp.Header.Values("Set-Cookie")) {
		// The server accepted the refresh but issued nothing new; the old
		// cookies are still the best we have and are evidently not working.
		return "", false
	}
	if err := store.Save(path); err != nil {
		// Losing the write is not fatal for this request - the refreshed
		// cookies are still usable in memory - but the next process would have
		// to refresh again, so it is worth saying so.
		fmt.Fprintf(os.Stderr, "warning: could not save the refreshed Trenda session: %v\n", err)
	}
	header = store.CookieHeader()
	if header == "" {
		return "", false
	}
	c.Config.TrendaSession = header
	return header, true
}
