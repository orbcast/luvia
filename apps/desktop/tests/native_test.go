//go:build e2e

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/locales"
)

// TestNativeShell drives a separately launched, isolated native test build.
// See ../README.md for the matching MyGo CLI and environment variables.
func TestNativeShell(t *testing.T) {
	cli, socket, data := os.Getenv("LUVIA_E2E_CLI"), os.Getenv("LUVIA_E2E_SOCKET"), os.Getenv("LUVIA_E2E_DATA_DIR")
	if cli == "" || socket == "" || data == "" {
		t.Fatal("set LUVIA_E2E_CLI, LUVIA_E2E_SOCKET and LUVIA_E2E_DATA_DIR for an isolated running test app")
	}
	// Wait for the actual socket, rather than assuming startup took long enough.
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		conn, err := net.DialTimeout("unix", socket, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("native app did not create its automation socket")
		}
	}
	session := fmt.Sprintf("luvia-native-%d", os.Getpid())
	run := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, cli, args...)
		cmd.Env = append(os.Environ(), "MYGO_AUTOMATION_SESSION="+session)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("mygo-automation %v: %v\n%s", args, err, out)
		}
		return out
	}
	module := exec.Command("go", "list", "-m", "-f={{.Version}}", "github.com/egoist/mygo")
	version, err := module.Output()
	if err != nil || strings.TrimSpace(string(run("version"))) != strings.TrimSpace(string(version)) {
		t.Fatalf("automation CLI must match the pinned MyGo module: %s (%v)", version, err)
	}
	run("open", "--socket", socket)
	t.Cleanup(func() { run("close") })
	assertPage := func(want appshell.Page, messages locales.Messages) {
		t.Helper()
		var tree struct {
			Nodes []struct{ Role, Label, Current string }
		}
		if err := json.Unmarshal(run("snapshot", "--json"), &tree); err != nil {
			t.Fatal(err)
		}
		buttons, current := 0, 0
		for _, node := range tree.Nodes {
			if node.Role == "text" {
				t.Fatal("native business content must remain empty")
			}
			if node.Role == "button" {
				buttons++
			}
			if node.Current == "page" {
				current++
				if node.Label != want.Label(messages) {
					t.Fatalf("current page %q, want %q", node.Label, want.Label(messages))
				}
			}
		}
		if buttons != 5 || current != 1 {
			t.Fatalf("native navigation has %d buttons and %d current pages", buttons, current)
		}
	}
	assertPage(appshell.Discover, english)
	for i, page := range appshell.Pages() {
		run("press", fmt.Sprintf("key=%d", i), "--settle")
		assertPage(page, english)
		run("menu", english.ViewMenu(), appshell.Discover.Label(english), "--settle")
		assertPage(appshell.Discover, english)
		run("key", fmt.Sprintf("CmdOrCtrl+%d", i+1), "--settle")
		assertPage(page, english)
		run("menu", english.ViewMenu(), page.Label(english), "--settle")
		assertPage(page, english)
	}
	run("key", "CmdOrCtrl+,", "--settle")
	assertPage(appshell.Settings, english)
	for _, theme := range []struct{ label, source string }{
		{english.Dark(), "dark"}, {english.Light(), "light"}, {english.FollowSystem(), "system"},
	} {
		run("menu", english.ViewMenu(), english.Appearance(), theme.label, "--settle")
		var preference struct{ Source string }
		content, err := os.ReadFile(filepath.Join(data, "theme.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(content, &preference); err != nil || preference.Source != theme.source {
			t.Fatalf("native theme preference: %s (%v), want %s", content, err, theme.source)
		}
		assertPage(appshell.Settings, english)
	}
	run("menu", english.ViewMenu(), english.Language(), english.SimplifiedChinese(), "--settle")
	chinese := locales.In("zh-Hans")
	assertPage(appshell.Settings, chinese)
	run("menu", chinese.ViewMenu(), appshell.Library.Label(chinese), "--settle")
	assertPage(appshell.Library, chinese)
	run("menu", chinese.ViewMenu(), chinese.Language(), chinese.English(), "--settle")
	assertPage(appshell.Library, english)
	run("menu", "Luvia", english.SettingsMenu(), "--settle")
	assertPage(appshell.Settings, english)
}
