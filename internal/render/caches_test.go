package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func fixtureCachesResult() CachesResult {
	return CachesResult{
		Repo: "indrasvat/gh-hound",
		Usage: &CacheUsage{
			ActiveSizeInBytes: 7730941133,
			ActiveCount:       11,
			CapBytes:          10737418240,
		},
		Caches: []Cache{
			{
				ID:             4902531779,
				Key:            "setup-go-Linux-x64-ubuntu24-go-1.26.4-d93f4ea3",
				Ref:            "refs/heads/main",
				SizeInBytes:    302526514,
				LastAccessedAt: time.Date(2026, 6, 11, 3, 46, 44, 0, time.UTC),
				CreatedAt:      time.Date(2026, 6, 10, 0, 59, 46, 0, time.UTC),
			},
		},
	}
}

func TestWriteCachesJSONShape(t *testing.T) {
	var out bytes.Buffer
	if err := WriteCaches(&out, FormatJSON, fixtureCachesResult()); err != nil {
		t.Fatalf("WriteCaches JSON error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON invalid: %v\n%s", err, out.String())
	}
	usage := decoded["usage"].(map[string]any)
	for _, key := range []string{"active_size_in_bytes", "active_count", "cap_bytes"} {
		if _, ok := usage[key]; !ok {
			t.Fatalf("usage missing %q in %s", key, out.String())
		}
	}
	cache := decoded["caches"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "key", "ref", "size_in_bytes", "last_accessed_at", "created_at"} {
		if _, ok := cache[key]; !ok {
			t.Fatalf("cache missing %q in %s", key, out.String())
		}
	}
	if _, ok := decoded["deleted"]; ok {
		t.Fatal("deleted must be omitted on pure listings")
	}
}

func TestWriteCachesDeletionEnvelope(t *testing.T) {
	result := CachesResult{
		Repo: "indrasvat/gh-hound",
		Deleted: &CacheDeletion{
			Action:       "delete_key",
			Key:          "go-mod",
			Ref:          "refs/heads/main",
			Accepted:     true,
			DeletedCount: 2,
		},
	}
	var out bytes.Buffer
	if err := WriteCaches(&out, FormatJSON, result); err != nil {
		t.Fatalf("WriteCaches JSON error: %v", err)
	}
	var decoded struct {
		Caches  []any `json:"caches"`
		Deleted struct {
			Action       string `json:"action"`
			Accepted     bool   `json:"accepted"`
			DeletedCount int    `json:"deleted_count"`
		} `json:"deleted"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON invalid: %v\n%s", err, out.String())
	}
	if decoded.Caches == nil {
		t.Fatalf("caches must marshal as [] not null:\n%s", out.String())
	}
	if decoded.Deleted.Action != "delete_key" || !decoded.Deleted.Accepted || decoded.Deleted.DeletedCount != 2 {
		t.Fatalf("deletion envelope wrong:\n%s", out.String())
	}
}

func TestWriteCachesRefusalCarriesTypedError(t *testing.T) {
	result := CachesResult{
		Repo: "indrasvat/gh-hound",
		Deleted: &CacheDeletion{
			Action:   "delete_key",
			Key:      "ghost",
			Accepted: false,
		},
		Error: &MutationError{Kind: "not_found", Message: "no caches matched key \"ghost\""},
	}
	var out bytes.Buffer
	if err := WriteCaches(&out, FormatJSON, result); err != nil {
		t.Fatalf("WriteCaches JSON error: %v", err)
	}
	if !strings.Contains(out.String(), `"kind": "not_found"`) {
		t.Fatalf("refusal must carry typed error kind:\n%s", out.String())
	}
}

func TestWriteCachesMarkdownSpeaksKennel(t *testing.T) {
	var out bytes.Buffer
	if err := WriteCaches(&out, FormatMarkdown, fixtureCachesResult()); err != nil {
		t.Fatalf("WriteCaches md error: %v", err)
	}
	if !strings.Contains(out.String(), "kennel: 7.2/10 GB") {
		t.Fatalf("markdown must carry the kennel usage header:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "| Key | Ref | Size | Last used |") {
		t.Fatalf("markdown table header missing:\n%s", out.String())
	}
}

func TestWriteCachesXMLIsWellFormed(t *testing.T) {
	var out bytes.Buffer
	if err := WriteCaches(&out, FormatXML, fixtureCachesResult()); err != nil {
		t.Fatalf("WriteCaches xml error: %v", err)
	}
	if !strings.Contains(out.String(), "<caches_result") || !strings.Contains(out.String(), "<cache ") {
		t.Fatalf("XML structure missing:\n%s", out.String())
	}
}

func TestHumanCacheGBFormatting(t *testing.T) {
	if got := humanCacheGB(7730941133); got != "7.2" {
		t.Fatalf("humanCacheGB = %q, want 7.2", got)
	}
	if got := humanCacheGB(10737418240); got != "10" {
		t.Fatalf("humanCacheGB cap = %q, want 10", got)
	}
}

// TestSchemaPublishesCachesContract pins the public contract: the
// caches envelope must ship in schema.json with the typed delete
// error including not_found.
func TestSchemaPublishesCachesContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema.json invalid: %v", err)
	}
	caches, ok := schema.Defs["caches_result"]
	if !ok {
		t.Fatal("schema.json must define $defs.caches_result")
	}
	for _, needle := range []string{"deleted_count", "active_size_in_bytes", "cap_bytes", "not_found"} {
		if !strings.Contains(string(caches), needle) {
			t.Fatalf("caches_result schema missing %q", needle)
		}
	}
}
