// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The annotation that hides a command from the MCP surface is "mcp:hidden".
// A misspelling is silent: the command keeps working on the command line and
// quietly stays available to agents. These tests pin the commands whose MCP
// visibility is a deliberate decision rather than an accident.
func TestMCPVisibilityOfWritingCommands(t *testing.T) {
	root := RootCmd()

	cases := []struct {
		path   string
		hidden bool
		why    string
	}{
		{"wod unplan", true,
			"deleting a planned workout removes it for everyone on the track"},
		{"auth login", true,
			"signing in reads a password and must never be driven by an agent"},
		{"auth set-widget-key", true, "stores a gym credential"},
		{"wod plan", false,
			"planning is the point of the CLI; it is guarded by --yes, not by hiding"},
		{"wod today", false, "reading is safe"},
		{"wod tracks", false, "reading is safe"},
	}

	for _, c := range cases {
		cmd := findCommand(root, c.path)
		if cmd == nil {
			t.Errorf("command %q not found", c.path)
			continue
		}
		got := strings.TrimSpace(cmd.Annotations["mcp:hidden"]) == "true"
		if got != c.hidden {
			t.Errorf("%q: mcp:hidden = %v, want %v (%s)", c.path, got, c.hidden, c.why)
		}
		// Guard against the specific typo this test was written for.
		if _, wrong := cmd.Annotations["mcp:exclude"]; wrong {
			t.Errorf("%q uses the annotation \"mcp:exclude\", which nothing reads; "+
				"the walker only honours \"mcp:hidden\"", c.path)
		}
	}
}

// wod plan stays visible to agents, so its refusal to write without --yes is
// the only thing standing between an agent and the gym's calendar.
func TestWodPlanRequiresExplicitConsentFlag(t *testing.T) {
	cmd := findCommand(RootCmd(), "wod plan")
	if cmd == nil {
		t.Fatal("wod plan not found")
	}
	if cmd.Flags().Lookup("yes") == nil {
		t.Error("wod plan has no --yes flag; nothing forces an explicit decision")
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("wod plan has no --dry-run flag; there is no way to preview a write")
	}
}

func findCommand(root *cobra.Command, path string) *cobra.Command {
	cur := root
	for _, name := range strings.Fields(path) {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}
