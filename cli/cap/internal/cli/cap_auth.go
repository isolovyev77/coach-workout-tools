// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Signing in to CrossFit with an email and password, the same
// shape of command the BTWB and Trenda CLIs offer.
//
// CrossFit uses OAuth2 with PKCE rather than a plain login form, but the whole
// exchange can be driven from a terminal: the sign-in endpoint takes the
// credentials directly and answers with an authorization code, which is then
// traded for an access token. No browser, no developer tools, no copying values
// out of a page.
//
// Two details were found by probing the API and are load bearing:
//
//   - The OAuth parameters (response_type, code_challenge, client_id,
//     redirect_uri) go in the QUERY STRING even though the credentials go in
//     the body. Without them the endpoint answers "Missing query parameter
//     'response_type'".
//   - The body must carry a country and a language, or the answer is
//     COUNTRY_AND_LANGUAGE_REQUIRED. "US"/"en" is accepted; "RU" is answered
//     with "Content not found", so the country is not a user preference here -
//     it selects a content region, and only some are served.

package cli

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"cap-pp-cli/internal/config"
)

// crossfitAPI is a var, not a const, so tests can point the sign-in at a replay
// server; nothing else reassigns it.
var crossfitAPI = "https://c3po.crossfit.com/api"

const (
	// The affiliate toolkit's public client. A public client holds no secret,
	// which is why PKCE is required.
	toolkitClientID    = "react_affiliate_toolkit_hBwg8A"
	toolkitRedirectURI = "https://affiliate.crossfit.com/tools/redirect"
	toolkitScope       = "user:full:read"

	// The content region the API serves this client. Not the user's country:
	// "RU" is rejected outright with "Content not found".
	signinCountry  = "US"
	signinLanguage = "en"
)

func newCapAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var email string
	var passwordStdin bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to CrossFit with your email and password",
		Long: `Sign in to CrossFit and store the resulting access token.

The password is sent only to CrossFit and is never written to disk; only the
token it returns is saved, in the config file with owner-only permissions.

The password is read from the terminal without echo. For unattended use, pass
--password-stdin and pipe it in, or set CROSSFIT_PASSWORD.

The token is short lived. When commands start failing with exit code 4, run
this again. The movement and benchmark commands need no token at all.`,
		Example: `  cap-pp-cli auth login
  cap-pp-cli auth login --email me@example.com`,
		Annotations: map[string]string{"mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}

			if email == "" {
				email = os.Getenv("CROSSFIT_EMAIL")
			}
			if email == "" {
				if flags.noInput {
					return usageErr(fmt.Errorf("--email is required with --no-input"))
				}
				fmt.Fprint(cmd.ErrOrStderr(), "CrossFit email: ")
				line, rErr := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if rErr != nil {
					return usageErr(fmt.Errorf("reading email: %w", rErr))
				}
				email = strings.TrimSpace(line)
			}
			if email == "" {
				return usageErr(fmt.Errorf("email is required"))
			}

			password, err := readCrossFitPassword(cmd, passwordStdin, flags.noInput)
			if err != nil {
				return err
			}
			if password == "" {
				return usageErr(fmt.Errorf("password is required"))
			}

			token, refresh, expiry, err := crossfitSignIn(email, password, flags.timeout)
			password = "" // done with it
			if err != nil {
				return err
			}

			cfg.AuthHeaderVal = "" // see auth set-token: a stale header would shadow the token
			if err := cfg.SaveTokens("", "", token, refresh, expiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}

			out := map[string]any{"signed_in": true, "config_path": cfg.Path}
			if !expiry.IsZero() {
				out["expires_at"] = expiry.Format(time.RFC3339)
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Signed in. Token stored in %s\n", cfg.Path)
			if !expiry.IsZero() {
				fmt.Fprintf(cmd.OutOrStdout(), "Valid until %s\n", expiry.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "CrossFit account email (or CROSSFIT_EMAIL)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false,
		"Read the password from stdin instead of prompting")
	return cmd
}

// readCrossFitPassword gets the password without ever echoing or storing it.
func readCrossFitPassword(cmd *cobra.Command, fromStdin, noInput bool) (string, error) {
	if v := os.Getenv("CROSSFIT_PASSWORD"); v != "" {
		return v, nil
	}
	if fromStdin {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", usageErr(fmt.Errorf("reading password from stdin: %w", err))
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}
	if noInput {
		return "", usageErr(fmt.Errorf(
			"no password available: use --password-stdin or set CROSSFIT_PASSWORD with --no-input"))
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", usageErr(fmt.Errorf(
			"stdin is not a terminal: use --password-stdin or set CROSSFIT_PASSWORD"))
	}
	fmt.Fprint(cmd.ErrOrStderr(), "CrossFit password: ")
	raw, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", usageErr(fmt.Errorf("reading password: %w", err))
	}
	return string(raw), nil
}

// crossfitSignIn performs the whole PKCE exchange and returns the tokens.
func crossfitSignIn(email, password string, timeout time.Duration) (
	accessToken, refreshToken string, expiry time.Time, err error) {

	verifier, challenge, err := newPKCEPair()
	if err != nil {
		return "", "", time.Time{}, err
	}
	client := &http.Client{
		Timeout: timeout,
		// The code comes back in a redirect the CLI must read, not follow:
		// following it would hand the code to the website instead.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	code, err := requestAuthCode(client, email, password, challenge)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return exchangeCode(client, code, verifier)
}

// newPKCEPair generates the verifier kept secret by this process and the
// challenge sent to the server, per RFC 7636.
func newPKCEPair() (verifier, challenge string, err error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generating the PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// oauthQuery is the parameter set the sign-in and authorize endpoints expect in
// the query string.
func oauthQuery(challenge string) string {
	q := url.Values{
		"response_type":         {"code"},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
		"client_id":             {toolkitClientID},
		"redirect_uri":          {toolkitRedirectURI},
		"scope":                 {toolkitScope},
	}
	return q.Encode()
}

// requestAuthCode posts the credentials and digs the authorization code out of
// whichever place the server puts it.
func requestAuthCode(client *http.Client, email, password, challenge string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
		"country":  signinCountry,
		"language": signinLanguage,
	})
	if err != nil {
		return "", fmt.Errorf("building the sign-in request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost,
		crossfitAPI+"/users/v2/auth/signin?"+oauthQuery(challenge),
		strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("building the sign-in request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cap-pp-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", apiErr(fmt.Errorf("signing in: %w", err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= 400 {
		return "", signInError(resp.StatusCode, data)
	}
	if code := codeFromLocation(resp.Header.Get("Location")); code != "" {
		return code, nil
	}
	if code := codeFromBody(data); code != "" {
		return code, nil
	}
	return "", apiErr(fmt.Errorf(
		"signed in but CrossFit returned no authorization code (HTTP %d); "+
			"the sign-in API may have changed", resp.StatusCode))
}

// signInError turns the API's error vocabulary into something a coach can act
// on, and maps a rejected password to exit code 4 rather than a generic failure.
func signInError(status int, data []byte) error {
	var payload struct {
		Error       string `json:"error"`
		Description string `json:"description"`
		Detail      string `json:"detail"`
	}
	_ = json.Unmarshal(data, &payload)

	switch payload.Error {
	case "CREDENTIALS_INVALID":
		return authErr(fmt.Errorf("CrossFit rejected that email and password"))
	case "COUNTRY_AND_LANGUAGE_REQUIRED":
		return apiErr(fmt.Errorf(
			"CrossFit now wants a different country/language pair than %s/%s",
			signinCountry, signinLanguage))
	}
	if payload.Error != "" {
		return authErr(fmt.Errorf("CrossFit refused the sign-in: %s", payload.Error))
	}
	detail := strings.TrimSpace(payload.Description + " " + payload.Detail)
	if detail == "" {
		detail = strings.TrimSpace(string(data))
	}
	if len(detail) > 200 {
		detail = detail[:200]
	}
	return apiErr(fmt.Errorf("CrossFit answered HTTP %d to the sign-in: %s", status, detail))
}

func codeFromLocation(location string) string {
	if location == "" {
		return ""
	}
	u, err := url.Parse(location)
	if err != nil {
		return ""
	}
	return u.Query().Get("code")
}

// codeFromBody reads the code out of a JSON answer. The field name is not
// guaranteed, so the few plausible spellings are all accepted.
func codeFromBody(data []byte) string {
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return ""
	}
	for _, key := range []string{"code", "authorization_code", "auth_code"} {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}
	// Some deployments nest the redirect under a "redirect_uri"/"location" key.
	for _, key := range []string{"redirect_uri", "redirectUri", "location", "url"} {
		if v, ok := payload[key].(string); ok {
			if code := codeFromLocation(v); code != "" {
				return code
			}
		}
	}
	return ""
}

// exchangeCode trades the authorization code for the token pair.
//
// The body goes out as JSON first because that is what the toolkit's own
// frontend sends, and the frontend receives a refresh_token where our earlier
// form-encoded exchange came back with none - the working theory for the empty
// refresh_token this CLI used to store. Form encoding is kept as a fallback
// since it demonstrably yielded access tokens.
func exchangeCode(client *http.Client, code, verifier string) (
	accessToken, refreshToken string, expiry time.Time, err error) {

	grant := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": verifier,
		"client_id":     toolkitClientID,
		"redirect_uri":  toolkitRedirectURI,
	}

	jsonBody, err := json.Marshal(grant)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("building the token request: %w", err)
	}
	access, refresh, expiry, jsonErr := postTokenRequest(client,
		strings.NewReader(string(jsonBody)), "application/json")
	if jsonErr == nil {
		return access, refresh, expiry, nil
	}

	form := url.Values{}
	for k, v := range grant {
		form.Set(k, v)
	}
	access, refresh, expiry, formErr := postTokenRequest(client,
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if formErr == nil {
		return access, refresh, expiry, nil
	}
	// The JSON attempt is the primary one; its error is the one to report.
	return "", "", time.Time{}, jsonErr
}

// postTokenRequest performs one attempt against the token endpoint and parses
// the answer. Shared by the JSON-primary and form-fallback paths.
func postTokenRequest(client *http.Client, body io.Reader, contentType string) (
	accessToken, refreshToken string, expiry time.Time, err error) {

	req, err := http.NewRequest(http.MethodPost, crossfitAPI+"/users/v2/auth/token", body)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("building the token request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cap-pp-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", time.Time{}, apiErr(fmt.Errorf("exchanging the code for a token: %w", err))
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
		return "", "", time.Time{}, authErr(fmt.Errorf(
			"CrossFit would not issue a token: %s", reason))
	}
	if payload.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	return payload.AccessToken, payload.RefreshToken, expiry, nil
}
