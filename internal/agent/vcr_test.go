package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/x/vcr"
	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v4"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

var testRecorderHeadersToKeep = map[string]struct{}{
	"accept":       {},
	"content-type": {},
	"user-agent":   {},
}

const salameyCompatibilityNote = "Use SALAMEY for the product name, but keep compatibility identifiers such as the `crush` command, `crush.json`, `.crush`, `CRUSH_*` environment variables, existing URLs, and package names unchanged unless the user explicitly asks otherwise.\n\n"

func newTestRecorder(t *testing.T) *vcr.Recorder {
	cassetteName := filepath.Join("testdata", t.Name())
	r, err := recorder.New(
		cassetteName,
		recorder.WithMode(recorder.ModeRecordOnce),
		recorder.WithMatcher(testRecorderMatcher(t)),
		recorder.WithMarshalFunc(testRecorderMarshaler),
		recorder.WithSkipRequestLatency(true),
		recorder.WithHook(testRecorderRemoveHeaders(false), recorder.AfterCaptureHook),
	)
	if err != nil {
		t.Fatalf("test vcr: failed to create recorder: %v", err)
	}

	t.Cleanup(func() {
		if err := r.Stop(); err != nil {
			t.Errorf("test vcr: failed to stop recorder: %v", err)
		}
	})

	return r
}

func testRecorderMatcher(t *testing.T) recorder.MatcherFunc {
	return func(r *http.Request, i cassette.Request) bool {
		if r.Body == nil || r.Body == http.NoBody {
			return cassette.DefaultMatcher(r, i)
		}
		if r.Method != i.Method || r.URL.String() != i.URL {
			return false
		}

		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("test vcr: failed to read request body: %v", err)
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(reqBody))

		requestContent := normalizeRecorderLineEndings(reqBody)
		cassetteContent := normalizeRecorderLineEndings(i.Body)
		if requestContent == cassetteContent {
			return true
		}

		requestContent, err = normalizeRecorderJSON(requestContent)
		if err != nil {
			printRecorderDiff(t, requestContent, cassetteContent)
			return false
		}
		cassetteContent, err = normalizeRecorderJSON(cassetteContent)
		if err != nil {
			printRecorderDiff(t, requestContent, cassetteContent)
			return false
		}
		if requestContent != cassetteContent {
			printRecorderDiff(t, requestContent, cassetteContent)
			return false
		}
		return true
	}
}

func normalizeRecorderJSON(input string) (string, error) {
	var payload any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return "", err
	}
	payload = normalizeRecorderValue(payload)
	normalized, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func normalizeRecorderValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, entry := range typed {
			if key == "cache_control" {
				continue
			}
			normalized[key] = normalizeRecorderValue(entry)
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, entry := range typed {
			normalized = append(normalized, normalizeRecorderValue(entry))
		}
		return normalized
	case string:
		return normalizeRecorderString(typed)
	default:
		return typed
	}
}

func normalizeRecorderString(input string) string {
	replacer := strings.NewReplacer(
		salameyCompatibilityNote, "",
		"- Respects .salameyignore patterns as a SALAMEY-compatible alias\n", "",
		"- Respects .crushignore patterns as a Crush-compatible alias\n", "",
		"- Respects .crushignore as a compatibility alias\n", "",
		"salamey@users.noreply.github.com", "crush@charm.land",
		".salameyignore", ".crushignore",
		"SALAMEY.md", "CRUSH.md",
		"salamey.json", "crush.json",
		"salamey.db", "crush.db",
	)
	normalized := replacer.Replace(normalizeRecorderLineEndings(input))
	normalized = strings.ReplaceAll(normalized, "You are SALAMEY, a powerful AI Assistant that runs in the CLI.", "You are Crush, a powerful AI Assistant that runs in the CLI.")
	normalized = strings.ReplaceAll(normalized, "SALAMEY", "Crush")
	return normalized
}

func normalizeRecorderLineEndings[T string | []byte](value T) string {
	normalized := string(value)
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, `\r\n`, `\n`)
	return normalized
}

func printRecorderDiff(t *testing.T, requestContent, cassetteContent string) {
	t.Logf("Request interaction not found for %q.\nDiff:\n%s", t.Name(), cmp.Diff(cassetteContent, requestContent))
}

func testRecorderMarshaler(in any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	enc.CompactSeqIndent()
	if err := enc.Encode(in); err != nil {
		return nil, fmt.Errorf("test vcr: unable to encode to yaml: %w", err)
	}
	return buf.Bytes(), nil
}

func testRecorderRemoveHeaders(keepAll bool) recorder.HookFunc {
	return func(i *cassette.Interaction) error {
		if keepAll {
			return nil
		}
		for key := range i.Request.Headers {
			if _, ok := testRecorderHeadersToKeep[strings.ToLower(key)]; !ok {
				delete(i.Request.Headers, key)
			}
		}
		for key := range i.Response.Headers {
			if _, ok := testRecorderHeadersToKeep[strings.ToLower(key)]; !ok {
				delete(i.Response.Headers, key)
			}
		}
		return nil
	}
}
