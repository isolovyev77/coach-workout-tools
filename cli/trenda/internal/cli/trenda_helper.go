// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Finding the Node sign-in helper that `auth login` drives.
//
// PATH alone is not enough. A non-interactive process - cron, a LaunchAgent, an
// agent shelling out - gets a login shell that never sources ~/.zshrc, so the
// ~/bin or ~/.local/bin the installer configured is simply absent, and the
// helper "disappears" on exactly the machines where nobody is watching. The
// release archive has the same problem from the other side: it ships the helper
// as apps/trenda/trenda-auth.mjs with no wrapper on PATH at all.
//
// So the helper is looked for where it actually tends to be, and as a last
// resort the .mjs is run through node directly.

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// trendaHelperCommand returns the command that runs the sign-in helper, or an
// error naming what was looked for.
func trendaHelperCommand(args ...string) (string, []string, error) {
	if explicit := os.Getenv("TRENDA_AUTH_HELPER"); explicit != "" {
		if isExecutableFile(explicit) {
			return explicit, args, nil
		}
		return "", nil, fmt.Errorf(
			"TRENDA_AUTH_HELPER points at %s, which is not an executable file", explicit)
	}

	if path, err := exec.LookPath("trenda-auth"); err == nil {
		return path, args, nil
	}

	for _, candidate := range helperCandidates() {
		if isExecutableFile(candidate) {
			return candidate, args, nil
		}
	}

	// Last resort: the .mjs itself, run through node. This is what a freshly
	// unpacked release archive has before install.sh writes a wrapper.
	if script, ok := findHelperScript(); ok {
		if node, err := exec.LookPath("node"); err == nil {
			return node, append([]string{script}, args...), nil
		}
		return "", nil, fmt.Errorf(
			"found the Trenda sign-in helper at %s but Node.js is not installed", script)
	}

	return "", nil, fmt.Errorf(
		"the Trenda sign-in helper was not found: looked on PATH, in ~/bin, " +
			"~/.local/bin, next to this binary, and for apps/trenda/trenda-auth.mjs. " +
			"Run scripts/install.sh, or set TRENDA_AUTH_HELPER to its path")
}

// helperCandidates lists the wrapper locations the installer uses, plus the
// directory holding this binary (release archives keep both under bin/).
func helperCandidates() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, "bin", "trenda-auth"),
			filepath.Join(home, ".local", "bin", "trenda-auth"),
		)
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil {
			exe = resolved
		}
		out = append(out, filepath.Join(filepath.Dir(exe), "trenda-auth"))
	}
	return out
}

// findHelperScript looks for apps/trenda/trenda-auth.mjs relative to this
// binary, walking up out of bin/ the way a release archive is laid out.
func findHelperScript() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	for i := 0; i < 4; i++ {
		candidate := filepath.Join(dir, "apps", "trenda", "trenda-auth.mjs")
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}
