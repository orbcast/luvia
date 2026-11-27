package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// lateKeyRule signs with a secret taken from the environment, the same way the
// packaged rules do, so a test can tell whether a value seeded after the rule
// was loaded still reached the request.
const lateKeyRule = `{
	"schemaVersion": 1, "id": "late", "name": "Late", "baseURL": "%s",
	"sign": {"secret": "${env:LATE_KEY}", "message": "{method} {path}",
		"headers": [{"name": "x-signature", "value": "{signature}"}]},
	"home": {
		"steps": [{"op": "request", "as": "res", "path": "/home"}],
		"output": {"from": "res.items[*]", "fields": {"id": "id", "title": "title"},
			"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
	}
}`

// TestKeySeededAfterLoadReachesTheRule pins the reader's moment rather than the
// rule: an install seeds the key once, after the loader already failed to build
// the source, and the expectation is that the loaded rule picks the value up
// without being reloaded.
func TestKeySeededAfterLoadReachesTheRule(t *testing.T) {
	os.Unsetenv("LATE_KEY")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-signature") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"1","title":"ok"}]}`))
	}))
	defer server.Close()

	built := buildSource(t, strings.Replace(lateKeyRule, "%s", server.URL, 1), server.Client())

	if _, err := built.Home(context.Background()); err == nil {
		t.Fatal("没有密钥时 Home 不该成功")
	}

	t.Setenv("LATE_KEY", "seeded-later")
	home, err := built.Home(context.Background())
	if err != nil {
		t.Fatalf("种子写入环境后 Home 失败: %v", err)
	}
	if len(home.Sections) == 0 {
		t.Fatalf("种子生效后应有区块，得到: %+v", home)
	}
}
