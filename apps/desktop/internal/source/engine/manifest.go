package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/rule"
)

// maxWindowBytes caps one window's document. A rule that reads a file server it
// does not control should not be able to pull a whole title into memory because
// the address answered with something else.
const maxWindowBytes = 8 << 20

// Manifest returns the dated comments an upstream publishes for one episode,
// oldest first, capped at limit.
//
// A catalogue that ships its own timeline comments says where they live instead
// of inlining them: one manifest lists the windows a long episode was cut into,
// and each window is a file of its own on a file server. Reading them is two
// requests plus one per window, and the rule describes that shape with its
// manifest and segments blocks.
func (s *sourceImpl) Manifest(ctx context.Context, episode source.Episode, limit int) ([]source.Danmaku, error) {
	if s.rule.Manifest == nil || s.rule.Segments == nil {
		return nil, s.unsupported("manifest")
	}
	if limit <= 0 {
		limit = defaultCommentLimit
	}
	address, err := s.manifestAddress(ctx, episode)
	if err != nil {
		return nil, err
	}
	return s.readWindows(ctx, episode, address, limit)
}

// manifestAddress asks the catalogue where one episode's comments live.
func (s *sourceImpl) manifestAddress(ctx context.Context, episode source.Episode) (string, error) {
	block := s.rule.Manifest
	site := callSite{op: "manifest", step: "manifest.output.fields.source"}
	vars, err := s.runSteps(ctx, "manifest", block.Steps, map[string]any{
		"id": episode.Media.ID, "payload": episode.ID,
	})
	if err != nil {
		return "", err
	}
	// The step is bound as `source`, so its fields are read under that name:
	// the block's own output says where the manifest is, and the `source`
	// field inside it is the address.
	value, ok := s.eval(field(block.Output.Fields, "source"), scope{vars: vars})
	if !ok {
		return "", s.fail(site, source.ErrInvalidResponse, "manifest names no address", nil)
	}
	address := strings.TrimSpace(toString(value))
	if address == "" {
		return "", s.fail(site, source.ErrInvalidResponse, "manifest names an empty address", nil)
	}
	return address, nil
}

// readWindows fetches the manifest and reads its windows until limit comments
// are collected or the list is used up.
//
// The manifest is the only document the caller cannot skip: it is what names
// every other one. A window that fails to arrive is skipped instead, because
// the comments already collected are worth showing and one bad file says little
// about the rest.
func (s *sourceImpl) readWindows(ctx context.Context, episode source.Episode, manifest string, limit int) ([]source.Danmaku, error) {
	block := s.rule.Segments
	spec := block.List
	site := callSite{op: "segments", step: "segments.list"}

	document, err := s.fetchDocument(ctx, site, manifest, "segments.list")
	if err != nil {
		return nil, err
	}
	windows, found := s.selectWindows(spec.From, map[string]any{"document": document, "source": manifest})
	if !found {
		return nil, s.fail(callSite{op: "segments", step: "segments.list.from"},
			source.ErrInvalidResponse,
			fmt.Sprintf("%q is missing from the manifest", spec.From), nil)
	}
	if spec.MaxSegments > 0 && len(windows) > spec.MaxSegments {
		windows = windows[:spec.MaxSegments]
	}

	comments := make([]source.Danmaku, 0, limit)
	for index, window := range windows {
		if len(comments) >= limit {
			break
		}
		sc := itemScope(window, index+1, map[string]any{"source": manifest})
		address, ok := s.eval(spec.URL, sc)
		if !ok {
			continue
		}
		windowURL := resolveAgainst(manifest, strings.TrimSpace(toString(address)))
		if windowURL == "" {
			continue
		}
		body, err := s.fetchDocument(ctx, site, windowURL, "segments.list")
		if err != nil {
			continue
		}
		vars, err := s.runSteps(ctx, "segments", block.Steps, map[string]any{
			"id": episode.Media.ID, "payload": episode.ID,
			"source": windowURL, "document": body,
		})
		if err != nil {
			continue
		}
		items, err := s.collectRows("segments", block.Output.From, vars)
		if err != nil {
			continue
		}
		for itemIndex, item := range items {
			if len(comments) >= limit {
				break
			}
			sc := itemScope(item, itemIndex+1, vars)
			text := strings.TrimSpace(s.evalString(field(block.Output.Fields, "text"), sc))
			if text == "" {
				continue
			}
			if want := s.evalString(field(block.Output.Fields, "episode"), sc); want != "" && want != episode.ID {
				continue
			}
			comment := source.Danmaku{
				Text:   text,
				Author: s.evalString(field(block.Output.Fields, "author"), sc),
			}
			if value, ok := s.eval(field(block.Output.Fields, "at"), sc); ok {
				// `at` is a duration, so a field written in whole seconds keeps
				// its fraction instead of rounding to the nearest one.
				if seconds, ok := toFloat(value); ok {
					comment.At = time.Duration(seconds * float64(time.Second))
				}
			}
			if value, ok := s.eval(field(block.Output.Fields, "color"), sc); ok {
				if color, ok := toInt(value); ok {
					comment.Color = uint32(color.(int)) & 0xFFFFFF
				}
			}
			comments = append(comments, comment)
		}
	}
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].At < comments[j].At })
	return comments, nil
}

// selectWindows resolves the list selector. Its root is the block's variables,
// exactly like an output selector's, so a rule names the step that holds the
// manifest: "document.segments[*]" reads the window list out of the document
// the segments block downloaded.
func (s *sourceImpl) selectWindows(from string, vars map[string]any) ([]any, bool) {
	for _, candidate := range strings.Split(from, "|") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		selector, err := rule.ParseSelector(candidate)
		if err != nil {
			continue
		}
		if value, ok := selector.Resolve(vars); ok {
			return asItems(value), true
		}
	}
	return nil, false
}

// collectRows resolves an output selector without unwrapping one level of it.
//
// A window packs each comment into an array rather than an object, so the list
// the mapping walks is a list of arrays. collect would flatten that into bare
// scalars, which is right for a wildcard that joined one list per branch and
// wrong here.
func (s *sourceImpl) collectRows(op, from string, vars map[string]any) ([]any, error) {
	value, ok := resolveCandidates(from, vars)
	if !ok {
		return nil, s.fail(callSite{op: op, step: op + ".output.from"},
			source.ErrInvalidResponse, fmt.Sprintf("%q is missing from the window", from), nil)
	}
	if list, ok := value.([]any); ok {
		return list, nil
	}
	return []any{value}, nil
}

// fetchDocument downloads a document named by another document and decodes it
// whether or not the upstream honoured its own content-encoding claim.
//
// The usual case is a file called `.json.gz` served as plain JSON with no
// Content-Encoding at all, because the file server stores what it was given.
// Trusting the declaration would double-decode; ignoring it would break the day
// a server does compress. The leading bytes settle it.
func (s *sourceImpl) fetchDocument(ctx context.Context, site callSite, address, label string) (any, error) {
	request := rule.Request{Method: "GET", Path: address, Sign: boolPtr(false)}
	body, err := s.fetchBody(ctx, site, request, scope{vars: map[string]any{}})
	if err != nil {
		return nil, err
	}
	if len(body) > maxWindowBytes {
		return nil, s.fail(site, source.ErrInvalidResponse,
			fmt.Sprintf("%s: document is larger than %d bytes", label, maxWindowBytes), nil)
	}
	decoded, err := decodeMaybeCompressed(body)
	if err != nil {
		return nil, s.fail(site, source.ErrInvalidResponse,
			fmt.Sprintf("%s: %s", label, err.Error()), err)
	}
	return decoded, nil
}

// decodeMaybeCompressed reads a body that may have arrived compressed without
// saying so. gzip, zlib and raw deflate are the three an upstream uses.
func decodeMaybeCompressed(body []byte) (any, error) {
	payload := body
	switch {
	case len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b:
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, errors.New("document claims gzip but its header is unreadable")
		}
		defer reader.Close()
		unpacked, err := readLimited(reader)
		if err != nil {
			return nil, err
		}
		payload = unpacked
	case len(body) >= 2 && body[0] == 0x78 && zlibHeader(body[1]):
		reader, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, errors.New("document claims zlib but its header is unreadable")
		}
		defer reader.Close()
		unpacked, err := readLimited(reader)
		if err != nil {
			return nil, err
		}
		payload = unpacked
	}
	decoded, err := decodeJSON(payload)
	if err == nil {
		return decoded, nil
	}
	// Raw deflate carries no header to recognise. Trying it once is cheaper
	// than failing a window that happens to be packed that way.
	if len(body) > 0 && body[0] != '{' && body[0] != '[' {
		reader := flate.NewReader(bytes.NewReader(body))
		defer reader.Close()
		if unpacked, readErr := readLimited(reader); readErr == nil {
			if decoded, decodeErr := decodeJSON(unpacked); decodeErr == nil {
				return decoded, nil
			}
		}
	}
	return nil, err
}

func decodeJSON(body []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, errors.New("document is not JSON")
	}
	return decoded, nil
}

func readLimited(reader io.Reader) ([]byte, error) {
	unpacked, err := io.ReadAll(io.LimitReader(reader, maxWindowBytes))
	if err != nil {
		return nil, errors.New("compressed document is truncated")
	}
	return unpacked, nil
}

// zlibHeader reports whether b can be the second byte of a zlib header: deflate
// with its check bits making 0x78xx a multiple of 31.
func zlibHeader(b byte) bool {
	return (b&0x0f) == 8 && (int(b)<<8|0x78)%31 == 0
}

func boolPtr(value bool) *bool { return &value }

// resolveAgainst reads a possibly relative address against the document it was
// found in.
func resolveAgainst(base, address string) string {
	if address == "" {
		return ""
	}
	parsed, err := url.Parse(address)
	if err == nil && parsed.IsAbs() {
		return address
	}
	if err != nil {
		return ""
	}
	from, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return from.ResolveReference(parsed).String()
}
