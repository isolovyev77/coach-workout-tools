package cli

import "testing"

func TestAuthCommandIncludesLogin(t *testing.T) {
	cmd := newAuthCmd(&rootFlags{})
	for _, child := range cmd.Commands() {
		if child.Name() == "login" {
			return
		}
	}
	t.Fatal("auth command must expose a login subcommand")
}
