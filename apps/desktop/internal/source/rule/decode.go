package rule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Decode parses one rule strictly: unknown fields are rejected so a typo in a
// hand-written rule fails loudly instead of being silently ignored.
func Decode(data []byte) (*Rule, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var parsed Rule
	if err := dec.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode rule: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode rule: trailing content after the rule object")
	}
	parsed.applyDefaults()
	return &parsed, nil
}

// DecodeFile reads and decodes one rule file.
func DecodeFile(path string) (*Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parsed, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return parsed, nil
}

func (r *Rule) applyDefaults() {
	if r.HTTP.TimeoutMs == 0 {
		r.HTTP.TimeoutMs = DefaultTimeoutMs
	}
	if r.HTTP.Retries == nil {
		retries := DefaultRetries
		r.HTTP.Retries = &retries
	}
	if r.HTTP.Concurrency == 0 {
		r.HTTP.Concurrency = DefaultConcurrency
	}
	if r.HTTP.MaxPages == 0 {
		r.HTTP.MaxPages = DefaultMaxPages
	}
	if r.Sign != nil {
		if r.Sign.NonceBytes == 0 {
			r.Sign.NonceBytes = 16
		}
		if r.Sign.TimestampUnit == "" {
			r.Sign.TimestampUnit = "ms"
		}
	}
	if r.Resolve != nil && r.Resolve.Lines.MaxAttempts == 0 {
		r.Resolve.Lines.MaxAttempts = DefaultMaxAttempts
	}
	r.forEachBlock(func(steps []Step) {
		for i := range steps {
			if steps[i].Op != OpPaginate {
				continue
			}
			if steps[i].Size == 0 {
				steps[i].Size = DefaultPageSize
			}
			if steps[i].MaxPages == 0 {
				steps[i].MaxPages = r.HTTP.MaxPages
			}
		}
	})
}

// forEachBlock visits each declared block's step slice; elements stay
// addressable so defaults can be written back into the rule.
func (r *Rule) forEachBlock(fn func(steps []Step)) {
	blocks := [][]Step{}
	if r.Home != nil {
		blocks = append(blocks, r.Home.Steps)
	}
	if r.List != nil {
		blocks = append(blocks, r.List.Steps)
	}
	if r.Detail != nil {
		blocks = append(blocks, r.Detail.Steps)
	}
	if r.Episodes != nil {
		blocks = append(blocks, r.Episodes.Steps)
	}
	if r.Resolve != nil {
		blocks = append(blocks, r.Resolve.Steps)
	}
	for _, steps := range blocks {
		fn(steps)
	}
}
