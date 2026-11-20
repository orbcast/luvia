package videocache

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// slowServer serves one body in chunks, one chunk per release the test gives it,
// so the cache's transfer can be read while it moves.
type slowServer struct {
	body  []byte
	step  int
	moved chan struct{}
	next  chan struct{}
	// gone ends the handler's own signalling when the test stops watching for
	// chunks; a read that is still running would otherwise block on it.
	gone chan struct{}
}

func (s *slowServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isSizeProbe(r.Header.Get("Range")) {
		w.Header().Set("Content-Length", strconv.Itoa(len(s.body)))
		w.Header().Set("Content-Range", "bytes 0-0/"+strconv.Itoa(len(s.body)))
		w.WriteHeader(http.StatusPartialContent)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(s.body)))
	for i := 0; i*s.step < len(s.body); i++ {
		if i > 0 {
			select {
			case <-s.next:
			case <-s.gone:
				return
			}
		}
		end := min((i+1)*s.step, len(s.body))
		if _, err := w.Write(s.body[i*s.step : end]); err != nil {
			return
		}
		w.(http.Flusher).Flush()
		select {
		case s.moved <- struct{}{}:
		case <-s.gone:
			return
		}
	}
}

// progressServer starts a body of size bytes that arrives step bytes at a time.
func progressServer(t *testing.T, size, step int) (*httptest.Server, *slowServer) {
	t.Helper()
	s := &slowServer{body: bytes.Repeat([]byte("x"), size), step: step,
		moved: make(chan struct{}), next: make(chan struct{}), gone: make(chan struct{})}
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	return server, s
}

// readInBackground starts a read of url and waits until the cache holds the
// first chunk.
func readInBackground(t *testing.T, url string, s *slowServer) {
	t.Helper()
	go func() {
		resp, err := http.Get(url)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	<-s.moved
}

// letThrough releases the chunk the server is waiting to write, and waits until
// the cache has taken the bytes it already had, so a transfer read afterwards
// covers what the test released.
func letThrough(t *testing.T, s *slowServer) {
	t.Helper()
	select {
	case s.next <- struct{}{}:
	case <-time.After(10 * time.Second):
		t.Fatal("the transfer never asked for another chunk")
	}
	<-s.moved
	// The cache copies bytes into its file a moment after the server writes
	// them, and a transfer shows at most one frame per tick.
	time.Sleep(progressTick + 100*time.Millisecond)
}

// releaseRest lets the body finish so the read holding it ends before the cache
// and the server close.
func releaseRest(t *testing.T, s *slowServer) {
	t.Helper()
	for done := 0; done < len(s.body)/s.step; done++ {
		select {
		case s.next <- struct{}{}:
		case <-time.After(200 * time.Millisecond):
			return
		}
		select {
		case <-s.moved:
		case <-time.After(200 * time.Millisecond):
			return
		}
	}
}

func TestTransferReportsProgressAndSpeed(t *testing.T) {
	server, slow := progressServer(t, 4000, 1000)
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	cache.moveClock(time.Unix(1000, 0))
	defer releaseRest(t, slow)
	readInBackground(t, cache.ProxyURL(server.URL+"/clip.mp4"), slow)

	cache.moveClock(time.Unix(1001, 0))
	letThrough(t, slow)
	cache.moveClock(time.Unix(1001, 0))
	first := cache.Transfer()
	if first.Done <= 0 || first.Done >= 4000 {
		t.Fatalf("bytes moved %d, want a part of the object", first.Done)
	}
	if first.Rate <= 0 {
		t.Fatalf("rate %v, want a speed", first.Rate)
	}

	// A slower second carries the average down while the bytes moved keep
	// climbing: the speed belongs to the transfer, not to the last chunk.
	cache.moveClock(time.Unix(1005, 0))
	letThrough(t, slow)
	cache.moveClock(time.Unix(1005, 0))
	later := cache.Transfer()
	if later.Done <= first.Done {
		t.Fatalf("bytes moved %d, want more than %d", later.Done, first.Done)
	}
	if later.Rate >= first.Rate {
		t.Fatalf("rate %v, want less than %v after a slower second", later.Rate, first.Rate)
	}

	// A transfer that went quiet is over, and reports nothing at all.
	cache.moveClock(time.Unix(1030, 0))
	if quiet := cache.Transfer(); quiet != (Transfer{}) {
		t.Fatalf("quiet transfer %+v, want nothing to show", quiet)
	}
}

// A warm-up fetches bytes for an episode nobody has opened yet. Its transfer is
// the wrong one to read a wait's speed from, so it publishes nothing at all.
func TestTransferShowsNothingWhileWarming(t *testing.T) {
	server, slow := progressServer(t, 2000, 1000)
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	cache.moveClock(time.Unix(1000, 0))
	defer releaseRest(t, slow)
	cache.Prefetch(server.URL + "/warm.m3u8")
	<-slow.moved

	cache.moveClock(time.Unix(1001, 0))
	letThrough(t, slow)
	cache.moveClock(time.Unix(1001, 0))
	if transfer := cache.Transfer(); transfer != (Transfer{}) {
		t.Fatalf("transfer %+v, want nothing while unwatched", transfer)
	}
}

func TestTransferOfStoredObjectReportsNothing(t *testing.T) {
	server, _ := newMediaServer(t, map[string][]byte{"/movie.mp4": []byte("0123456789abc")})
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	status, _, body := get(t, cache.ProxyURL(server.URL+"/movie.mp4"))
	if status != http.StatusOK || len(body) != 13 {
		t.Fatalf("read: %d %q", status, body)
	}
	// The transfer took its record down once it stored the object, so a wait
	// that begins now has nothing of it to read back.
	if transfer := cache.Transfer(); transfer != (Transfer{}) {
		t.Fatalf("transfer %+v, want nothing once it settled", transfer)
	}
}

func TestTransferOfFailedFetchLeavesNothing(t *testing.T) {
	server, _ := newMediaServer(t, map[string][]byte{})
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	status, _, _ := get(t, cache.ProxyURL(server.URL+"/missing.mp4"))
	if status == http.StatusOK {
		t.Fatalf("read of a missing object answered %d", status)
	}
	if transfer := cache.Transfer(); transfer != (Transfer{}) {
		t.Fatalf("transfer %+v, want nothing after a failed fetch", transfer)
	}
}
