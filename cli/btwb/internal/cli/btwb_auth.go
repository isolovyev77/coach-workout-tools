// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. btwb has no OAuth and no personal access tokens: the only way
// in is the same form login a browser performs. This signs in once, keeps the
// resulting session cookie in the config file, and signs in again when btwb
// expires it.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"btwb-pp-cli/internal/config"
)

const (
	btwbSignInURL  = "https://btwb.com/signin"
	btwbSessionURL = "https://btwb.com/session"
	btwbHomeURL    = "https://btwb.com/whiteboard"

	// btwb renamed its Rails cookie in July 2026. Prefer the current name, but
	// continue accepting the legacy name so the login parser remains compatible
	// with older deployments and fixtures.
	btwbSessionCookieName       = "_btwb_session_id"
	btwbLegacySessionCookieName = "_btwb_session"
)

var (
	// Rails puts the CSRF token in the form; attribute order varies by page.
	csrfFormRe  = regexp.MustCompile(`name="authenticity_token"[^>]*value="([^"]+)"`)
	csrfFormRe2 = regexp.MustCompile(`value="([^"]+)"[^>]*name="authenticity_token"`)
	// The signed-in whiteboard links to the member it belongs to.
	memberIDRe = regexp.MustCompile(`/members/(\d+)`)
	// The sign-in page, which btwb serves whenever a session cannot read a page.
	signinPageRe = regexp.MustCompile(`<form[^>]+action="/session"`)
)

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var email string
	var passwordStdin bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to btwb and store the session locally",
		Long: `Sign in to btwb with your email and password.

btwb offers no API token, so this performs the same form login a browser does.
The password is sent only to btwb.com and is never written to disk; only the
resulting session cookie is stored, in the config file with mode 0600.

The password is read from the terminal without echo. For unattended use, pass
--password-stdin and pipe it in, or set BTWB_PASSWORD.`,
		Example: `  btwb-pp-cli auth login
  btwb-pp-cli auth login --email me@example.com
  pass btwb | btwb-pp-cli auth login --email me@example.com --password-stdin`,
		Annotations: map[string]string{"mcp:exclude": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}

			if email == "" {
				email = os.Getenv("BTWB_EMAIL")
			}
			if email == "" {
				if flags.noInput {
					return usageErr(fmt.Errorf("--email is required with --no-input"))
				}
				fmt.Fprint(cmd.ErrOrStderr(), "btwb email: ")
				line, rErr := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if rErr != nil {
					return usageErr(fmt.Errorf("reading email: %w", rErr))
				}
				email = strings.TrimSpace(line)
			}
			if email == "" {
				return usageErr(fmt.Errorf("email is required"))
			}

			password, err := readPassword(cmd, passwordStdin, flags.noInput)
			if err != nil {
				return err
			}
			if password == "" {
				return usageErr(fmt.Errorf("password is required"))
			}

			cookies, memberID, err := btwbFormLoginCookies(email, password, flags.timeout)
			// Drop the password as soon as it has been used.
			password = ""
			_ = password
			if err != nil {
				return err
			}

			if err := cfg.SaveSessionCookies(cookies, memberID); err != nil {
				return configErr(err)
			}

			out := map[string]any{
				"signed_in":   true,
				"member_id":   memberID,
				"config_path": cfg.Path,
			}
			if flags.asJSON {
				return flags.printJSON(cmd, out)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Signed in as member %d. Session stored in %s\n",
				memberID, cfg.Path)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "btwb account email (or BTWB_EMAIL)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false,
		"Read the password from stdin instead of prompting")
	return cmd
}

// readPassword gets the password without ever echoing or storing it.
func readPassword(cmd *cobra.Command, fromStdin, noInput bool) (string, error) {
	if v := os.Getenv("BTWB_PASSWORD"); v != "" {
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
			"no password available: use --password-stdin or set BTWB_PASSWORD with --no-input"))
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", usageErr(fmt.Errorf(
			"stdin is not a terminal: use --password-stdin or set BTWB_PASSWORD"))
	}
	fmt.Fprint(cmd.ErrOrStderr(), "btwb password: ")
	raw, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", usageErr(fmt.Errorf("reading password: %w", err))
	}
	return string(raw), nil
}

// btwbFormLogin performs the sign-in and returns the session cookie value and
// the member id it belongs to.
func btwbFormLogin(email, password string, timeout time.Duration) (string, int, error) {
	cookies, memberID, err := btwbFormLoginCookies(email, password, timeout)
	if err != nil {
		return "", 0, err
	}
	for _, name := range []string{btwbSessionCookieName, btwbLegacySessionCookieName} {
		if session := cookies[name]; session != "" {
			return session, memberID, nil
		}
	}
	return "", 0, authErr(fmt.Errorf("btwb returned no session cookie"))
}

// btwbFormLoginCookies performs the browser login and returns every cookie the
// browser would retain, including the long-lived remember-me credential.
func btwbFormLoginCookies(email, password string, timeout time.Duration) (map[string]string, int, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating cookie jar: %w", err)
	}
	httpClient := &http.Client{Timeout: timeout, Jar: jar}

	// Step 1: the sign-in page, for the CSRF token and the pre-login cookie.
	page, err := httpGetString(httpClient, btwbSignInURL)
	if err != nil {
		return nil, 0, fmt.Errorf("opening the btwb sign-in page: %w", err)
	}
	token := ""
	if m := csrfFormRe.FindStringSubmatch(page); m != nil {
		token = m[1]
	} else if m := csrfFormRe2.FindStringSubmatch(page); m != nil {
		token = m[1]
	}
	if token == "" {
		return nil, 0, fmt.Errorf("btwb's sign-in form changed: no authenticity_token found")
	}

	// Step 2: submit the form. remember_me asks btwb for a long-lived session.
	form := url.Values{
		"authenticity_token": {token},
		"login":              {email},
		"password":           {password},
		"remember_me":        {"1"},
		"commit":             {"Sign In"},
	}
	req, err := http.NewRequest(http.MethodPost, btwbSessionURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("building the sign-in request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://btwb.com")
	req.Header.Set("Referer", btwbSignInURL)
	req.Header.Set("User-Agent", "btwb-pp-cli")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("signing in: %w", err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	// A rejected sign-in still hands out a session cookie - Rails issues one for
	// every request - and answers 302 back to /signin. So the cookie proves
	// nothing; the only reliable test is whether the session can read the
	// member's own whiteboard.
	if sessionFromJar(jar) == "" {
		return nil, 0, authErr(fmt.Errorf("btwb returned HTTP %d without a session cookie",
			resp.StatusCode))
	}

	// Step 3: confirm the session works, and learn which member it belongs to.
	home, err := httpGetString(httpClient, btwbHomeURL)
	if err != nil {
		return nil, 0, fmt.Errorf("checking the new session: %w", err)
	}
	if signinPageRe.MatchString(home) {
		return nil, 0, authErr(fmt.Errorf("btwb rejected the email or password"))
	}
	m := memberIDRe.FindStringSubmatch(home)
	if m == nil {
		return nil, 0, authErr(fmt.Errorf(
			"signed in but btwb's whiteboard did not identify the member; " +
				"the page layout may have changed"))
	}
	memberID, _ := strconv.Atoi(m[1])
	return cookiesFromJar(jar), memberID, nil
}

func httpGetString(c *http.Client, target string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "btwb-pp-cli")
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return string(body), nil
}

func sessionFromJar(jar *cookiejar.Jar) string {
	cookies := cookiesFromJar(jar)
	for _, name := range []string{btwbSessionCookieName, btwbLegacySessionCookieName} {
		if session := cookies[name]; session != "" {
			return session
		}
	}
	return ""
}

func cookiesFromJar(jar *cookiejar.Jar) map[string]string {
	cookies := map[string]string{}
	u, err := url.Parse("https://btwb.com/")
	if err != nil {
		return cookies
	}
	for _, cookie := range jar.Cookies(u) {
		if cookie.Name != "" && cookie.Value != "" {
			cookies[cookie.Name] = cookie.Value
		}
	}
	return cookies
}

func newAuthSetWidgetKeyCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "set-widget-key <key>",
		Short: "Save the gym's Web Widgets key for the widget commands",
		Long: `Save the gym's Web Widgets key.

The widget commands read WODs, gym activity and leaderboards from btwb's Web
Widgets API, which is authenticated per gym rather than per member. A gym admin
finds the key under the gym menu -> Website Integration.

Members without admin rights cannot mint this key; the wod commands work
without it.`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"mcp:exclude": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			key := strings.TrimSpace(args[0])
			if key == "" {
				return usageErr(fmt.Errorf("key is empty"))
			}
			if err := cfg.SaveWidgetKey(key); err != nil {
				return configErr(err)
			}
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"widget_key_saved": true, "config_path": cfg.Path,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Widget key stored in %s\n", cfg.Path)
			return nil
		},
	}
}
