//go:build e2e

package main

import (
	"os"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/plugins/automation"
)

// Test builds can isolate framework state and theme preferences from user data.
func init() {
	if path := os.Getenv("LUVIA_E2E_DATA_DIR"); path != "" {
		mygo.App.SetPath(mygo.PathUserData, path)
	}
}
