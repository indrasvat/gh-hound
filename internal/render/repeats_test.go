package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func repeatsResult() Result {
	result := fixtureResult()
	result.Runs[0].Failed[0].Repeats = []Repeat{{
		StartLine: 8, EndLine: 1443, Cycles: 720, BlockLines: 2, HiddenLines: 1436,
		From: "16:12:11", To: "18:12:02", Summary: "×720 2-line block · 16:12:11 → 18:12:02 · 3 changing ids",
	}}
	return result
}

func TestRepeatsRenderInEveryFormat(t *testing.T) {
	var js bytes.Buffer
	if err := Write(&js, FormatJSON, repeatsResult()); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Runs []struct {
			Failed []struct {
				Repeats []map[string]any `json:"repeats"`
			} `json:"failed"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(js.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	r := decoded.Runs[0].Failed[0].Repeats[0]
	for _, key := range []string{"start_line", "end_line", "cycles", "block_lines", "hidden_lines", "from", "to", "summary"} {
		if _, ok := r[key]; !ok {
			t.Fatalf("repeat missing %q: %v", key, r)
		}
	}
	var md bytes.Buffer
	if err := Write(&md, FormatMarkdown, repeatsResult()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md.String(), "- L8–L1443: ×720 2-line block") || !strings.Contains(md.String(), "(1436 lines hidden)") {
		t.Fatalf("markdown repeats:\n%s", md.String())
	}
	var xml bytes.Buffer
	if err := Write(&xml, FormatXML, repeatsResult()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xml.String(), "<repeats>") || !strings.Contains(xml.String(), `<repeat start_line="8" end_line="1443" cycles="720" block_lines="2" hidden_lines="1436"`) {
		t.Fatalf("xml repeats:\n%s", xml.String())
	}
}

func TestRepeatsOmittedWhenEmptyAndInSchema(t *testing.T) {
	var js bytes.Buffer
	if err := Write(&js, FormatJSON, fixtureResult()); err != nil {
		t.Fatal(err)
	}
	var x bytes.Buffer
	if err := Write(&x, FormatXML, fixtureResult()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(js.String(), "repeats") || strings.Contains(x.String(), "repeats") {
		t.Fatal("repeats must be omitted when there are none")
	}
	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"repeats"`) || !strings.Contains(string(raw), `"hidden_lines"`) {
		t.Fatal("schema.json must describe repeats")
	}
}
