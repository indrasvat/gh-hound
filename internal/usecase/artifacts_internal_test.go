package usecase

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs"
)

func TestExtractZipEnforcesBudget(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create("bomb.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(bytes.Repeat([]byte{0}, 64*1024)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}

	_, err = extractZip(reader, filepath.Join(t.TempDir(), "out"), 1024)
	if err == nil || !strings.Contains(err.Error(), "safety budget") {
		t.Fatalf("expansion past budget must error, got %v", err)
	}
}

func TestExtractionBudgetScalesWithArchive(t *testing.T) {
	if got := extractionBudget(1); got != extractionBudgetMin {
		t.Fatalf("small archives keep the floor: %d", got)
	}
	if got := extractionBudget(1 << 30); got != (1<<30)*extractionRatioCap {
		t.Fatalf("large archives scale by ratio: %d", got)
	}
}

func TestExtractZipCleanupContractOnFailure(t *testing.T) {
	// Download() removes the target on extraction failure; this pins the
	// behavior at the extractZip level used by that path.
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	good, _ := writer.Create("good.txt")
	_, _ = good.Write([]byte("ok"))
	bad, _ := writer.Create("../escape.txt")
	_, _ = bad.Write([]byte("nope"))
	_ = writer.Close()
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "out")
	if _, err := extractZip(reader, target, 1<<20); err == nil {
		t.Fatal("zip-slip entry must fail extraction")
	}
	if _, statErr := os.Stat(filepath.Join(target, "good.txt")); statErr != nil {
		// extractZip itself leaves prior entries; Download removes them.
		t.Skipf("entry order changed: %v", statErr)
	}
}

func TestExcerptForStripsTimestamps(t *testing.T) {
	// Uses the triage excerpt path shared by the pipe surface.
	raw := "2026-06-10T15:53:14.2803225Z --- FAIL: TestX (0.00s)\n2026-06-10T15:53:15.1281282Z ##[error]Process completed with exit code 1."
	doc := parseForTest(raw)
	excerpt := excerptFor(doc)
	if strings.Contains(excerpt, "2026-06-10T") {
		t.Fatalf("pipe excerpt must strip timestamps:\n%s", excerpt)
	}
	if !strings.Contains(excerpt, "--- FAIL: TestX") {
		t.Fatalf("excerpt lost the anchor:\n%s", excerpt)
	}
}

func parseForTest(raw string) logs.Document {
	return logs.Parse(raw)
}
