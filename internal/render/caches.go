package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// CachesResult is the pipe envelope for the caches command: kennel
// usage against the effective cap, the cache listing, and (for the
// delete flags) the mutation outcome with the task-240 typed error.
type CachesResult struct {
	XMLName xml.Name       `json:"-" xml:"caches_result"`
	Repo    string         `json:"repo" xml:"repo,attr"`
	Usage   *CacheUsage    `json:"usage,omitempty" xml:"usage,omitempty"`
	Caches  []Cache        `json:"caches" xml:"caches>cache"`
	Deleted *CacheDeletion `json:"deleted,omitempty" xml:"deleted,omitempty"`
	Error   *MutationError `json:"error,omitempty" xml:"error,omitempty"`
}

type Cache struct {
	ID             int64     `json:"id" xml:"id,attr"`
	Key            string    `json:"key" xml:"key,attr"`
	Ref            string    `json:"ref" xml:"ref,attr"`
	SizeInBytes    int64     `json:"size_in_bytes" xml:"size_in_bytes,attr"`
	LastAccessedAt time.Time `json:"last_accessed_at" xml:"last_accessed_at,attr"`
	CreatedAt      time.Time `json:"created_at" xml:"created_at,attr"`
}

// CacheUsage carries the numbers agents need to do their own gauge
// math. cap_bytes is the effective eviction cap — the documented
// 10 GB fallback, since github.com exposes no cap endpoint.
type CacheUsage struct {
	ActiveSizeInBytes int64 `json:"active_size_in_bytes" xml:"active_size_in_bytes,attr"`
	ActiveCount       int   `json:"active_count" xml:"active_count,attr"`
	CapBytes          int64 `json:"cap_bytes" xml:"cap_bytes,attr"`
}

// CacheDeletion reports an eviction request. Action is delete_id or
// delete_key; key deletes can match several caches, so agents branch
// on deleted_count, never on prose.
type CacheDeletion struct {
	Action       string `json:"action" xml:"action,attr"`
	ID           int64  `json:"id,omitempty" xml:"id,attr,omitempty"`
	Key          string `json:"key,omitempty" xml:"key,attr,omitempty"`
	Ref          string `json:"ref,omitempty" xml:"ref,attr,omitempty"`
	Accepted     bool   `json:"accepted" xml:"accepted,attr"`
	DeletedCount int    `json:"deleted_count" xml:"deleted_count,attr"`
}

func WriteCaches(w io.Writer, format Format, result CachesResult) error {
	if result.Caches == nil {
		// Agents iterate caches[] unconditionally; an empty kennel is
		// [] and never null.
		result.Caches = []Cache{}
	}
	switch format {
	case "", FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case FormatMarkdown:
		return writeCachesMarkdown(w, result)
	case FormatXML:
		if _, err := fmt.Fprintln(w, xml.Header[:len(xml.Header)-1]); err != nil {
			return err
		}
		encoder := xml.NewEncoder(w)
		encoder.Indent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return err
		}
		_, err := fmt.Fprintln(w)
		return err
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func writeCachesMarkdown(w io.Writer, result CachesResult) error {
	if _, err := fmt.Fprintf(w, "# gh-hound caches\n\nRepo: `%s`\n", result.Repo); err != nil {
		return err
	}
	if usage := result.Usage; usage != nil {
		if _, err := fmt.Fprintf(w, "\n%s\n", KennelUsageLine(usage.ActiveSizeInBytes, usage.CapBytes)); err != nil {
			return err
		}
	}
	if len(result.Caches) > 0 {
		if _, err := fmt.Fprint(w, "\n| Key | Ref | Size | Last used |\n| --- | --- | --- | --- |\n"); err != nil {
			return err
		}
		for _, cache := range result.Caches {
			if _, err := fmt.Fprintf(w, "| %s | %s | %d | %s |\n", cache.Key, cache.Ref, cache.SizeInBytes, cache.LastAccessedAt.Format(time.RFC3339)); err != nil {
				return err
			}
		}
	}
	if deleted := result.Deleted; deleted != nil {
		if _, err := fmt.Fprintf(w, "\nDelete `%s`: accepted %t, dug up %d.\n", deleted.Action, deleted.Accepted, deleted.DeletedCount); err != nil {
			return err
		}
	}
	if result.Error != nil {
		if _, err := fmt.Fprintf(w, "\nError: `%s` — %s\n", result.Error.Kind, result.Error.Message); err != nil {
			return err
		}
	}
	return nil
}

// KennelUsageLine renders the hound-voiced usage header shared by the
// markdown surface and the TUI gauge: `kennel: 7.2/10 GB`.
func KennelUsageLine(activeBytes, capBytes int64) string {
	if capBytes <= 0 {
		capBytes = 10 << 30
	}
	return "kennel: " + humanCacheGB(activeBytes) + "/" + humanCacheGB(capBytes) + " GB"
}

// humanCacheGB renders GiB with one decimal, dropping the trailing
// .0 so the cap reads `10`, not `10.0`.
func humanCacheGB(bytes int64) string {
	value := float64(bytes) / float64(1<<30)
	formatted := strconv.FormatFloat(value, 'f', 1, 64)
	return strings.TrimSuffix(formatted, ".0")
}
