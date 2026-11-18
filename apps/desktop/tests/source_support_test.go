package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"luvia/apps/desktop/internal/source/engine"
	"luvia/apps/desktop/internal/source/loader"
)

// rulesDir is the checked-out rule directory; the loader reads it exactly the
// way a development run does through LUVIA_RULES_DIR.
const rulesDir = "../../luvia-rules"

// recordedRequest is the HTTP a rule asked the engine to send. Tests replace the
// transport, so nothing leaves the process and every request is inspectable.
type recordedRequest struct {
	Method string
	URL    string
	Path   string
	Query  url.Values
	Header http.Header
	Body   string
}

// RequestURI renders the path and query the way they appeared on the wire,
// which is also the string an upstream signs.
func (r recordedRequest) RequestURI() string {
	parsed, err := url.Parse(r.URL)
	if err != nil {
		return r.URL
	}
	return parsed.RequestURI()
}

// stubUpstream answers requests with canned bodies and keeps the requests.
type stubUpstream struct {
	handle  func(recordedRequest) (int, string)
	mu      sync.Mutex
	records []recordedRequest
}

func newStubUpstream(t *testing.T, handle func(recordedRequest) (int, string)) (*stubUpstream, *http.Client) {
	t.Helper()
	stub := &stubUpstream{handle: handle}
	client := &http.Client{Transport: stub}
	t.Cleanup(client.CloseIdleConnections)
	return stub, client
}

func (s *stubUpstream) RoundTrip(request *http.Request) (*http.Response, error) {
	body := ""
	if request.Body != nil {
		encoded, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		body = string(encoded)
	}
	record := recordedRequest{
		Method: request.Method,
		URL:    request.URL.String(),
		Path:   request.URL.Path,
		Query:  request.URL.Query(),
		Header: request.Header.Clone(),
		Body:   body,
	}
	s.mu.Lock()
	s.records = append(s.records, record)
	s.mu.Unlock()

	status, payload := s.handle(record)
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(payload)),
		Request:    request,
	}, nil
}

// Requests returns the captured requests in send order.
func (s *stubUpstream) Requests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedRequest, len(s.records))
	copy(out, s.records)
	return out
}

// readFixture loads one test file relative to the tests package.
func readFixture(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"."}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(data)
}

// loadSources loads a rule directory with the stub transport attached.
func loadSources(t *testing.T, dir string, client *http.Client) *loader.Set {
	t.Helper()
	set, err := loader.Load(dir, engine.Options{Client: client})
	if err != nil {
		t.Fatalf("load %s: %v", dir, err)
	}
	return set
}

// decodeBody parses a recorded JSON request body.
func decodeBody(t *testing.T, record recordedRequest) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(record.Body), &body); err != nil {
		t.Fatalf("request body %q is not JSON: %v", record.Body, err)
	}
	return body
}
