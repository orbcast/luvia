package danmaku

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestBilibiliLive asks the site for a title that is really there. It is opt-in
// so the suite stays offline: LUVIA_DANMAKU_LIVE=1 runs it.
func TestBilibiliLive(t *testing.T) {
	if os.Getenv("LUVIA_DANMAKU_LIVE") == "" {
		t.Skip("set LUVIA_DANMAKU_LIVE=1 to ask the site")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	lookup := NewBilibili(nil)
	got, err := lookup.Lookup(ctx, "全球死考：刚下刑场你让我去考试", 1)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	t.Logf("reason=%v items=%d", got.Reason, len(got.Items))
	if got.Reason != ReasonReady {
		t.Fatalf("reason = %v, want ready", got.Reason)
	}
	for i, item := range got.Items {
		if i >= 3 {
			break
		}
		t.Logf("%v %q colour=%#x", item.At, item.Text, item.Color)
	}
}
