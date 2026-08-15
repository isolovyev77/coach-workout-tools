// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// CrossFit's access token lives a few hours. The sign-in exchange also issues a
// refresh token, and the affiliate toolkit's own frontend keeps its session
// alive by posting it back to the same token endpoint - JSON body,
// grant_type "refresh_token", nothing else. This file does the same, so a coach
// signs in once and ordinary use renews the session on its own.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TokenURL is where CrossFit issues and refreshes the toolkit's tokens. A var,
// not a const, so tests can point the refresh at a replay server.
var TokenURL = "https://c3po.crossfit.com/api/users/v2/auth/token"

// refreshClientID is the public client id of the affiliate toolkit - the same
// one sign-in uses (cli/cap_auth.go), because a refresh token is bound to the
// client that obtained it.
const refreshClientID = "react_affiliate_toolkit_hBwg8A"

// refreshAccessToken trades the stored refresh token for a fresh pair and
// persists it. The caller decides when: proactively when the stored expiry has
// passed, or reactively when the API answers 401.
func (c *Client) refreshAccessToken() error {
	if c.Config == nil || c.Config.RefreshToken == "" {
		return fmt.Errorf("the session has expired and no refresh token is stored; run 'cap-pp-cli auth login'")
	}

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
		return fmt.Errorf("refreshing the CrossFit session: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	_ = json.Unmarshal(data, &payload)

	if resp.StatusCode >= 400 || payload.AccessToken == "" {
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
		// The refresh token has been used up or revoked; only a password can
		// restart the session from here.
		return fmt.Errorf("CrossFit declined to refresh the session (%s); run 'cap-pp-cli auth login'", reason)
	}

	// Rotation is the server's choice: keep the old refresh token unless a new
	// one arrived, or the next refresh would have nothing to send.
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
		return fmt.Errorf("the session was refreshed but could not be saved: %w", err)
	}
	return nil
}
