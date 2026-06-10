package usecase_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestArtifactsListReturnsRunArtifacts(t *testing.T) {
	service := usecase.ArtifactsService{GitHub: fake.New(fake.ScenarioFailing)}

	artifacts, err := service.List(context.Background(), "indrasvat/gh-hound", 30433642)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %d, want 2", len(artifacts))
	}
	if artifacts[0].Name != "coverage" || artifacts[0].SizeInBytes == 0 {
		t.Fatalf("first artifact = %#v", artifacts[0])
	}
	if !artifacts[1].Expired {
		t.Fatalf("second fixture artifact should be expired: %#v", artifacts[1])
	}
}

func TestArtifactsDownloadExtractsZipIntoNamedDir(t *testing.T) {
	service := usecase.ArtifactsService{GitHub: fake.New(fake.ScenarioFailing)}
	dest := t.TempDir()
	artifact := model.Artifact{ID: 901, Name: "coverage"}

	result, err := service.Download(context.Background(), "indrasvat/gh-hound", artifact, dest, false)
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	wantDir := filepath.Join(dest, "coverage")
	if result.Path != wantDir {
		t.Fatalf("result path = %q, want %q", result.Path, wantDir)
	}
	content, err := os.ReadFile(filepath.Join(wantDir, "coverage.out"))
	if err != nil {
		t.Fatalf("extracted file missing: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("extracted file is empty")
	}
	if _, err := os.Stat(filepath.Join(wantDir, "nested", "summary.json")); err != nil {
		t.Fatalf("nested extraction failed: %v", err)
	}
	if result.FileCount != 2 {
		t.Fatalf("file count = %d, want 2", result.FileCount)
	}
}

func TestArtifactsDownloadRejectsExpiredWithoutNetworkCall(t *testing.T) {
	github := &countingArtifactGitHub{Adapter: fake.New(fake.ScenarioFailing)}
	service := usecase.ArtifactsService{GitHub: github}
	artifact := model.Artifact{ID: 902, Name: "old-report", Expired: true}

	_, err := service.Download(context.Background(), "indrasvat/gh-hound", artifact, t.TempDir(), false)
	var expired usecase.ArtifactExpiredError
	if !errors.As(err, &expired) {
		t.Fatalf("error = %v, want ArtifactExpiredError", err)
	}
	if github.downloads != 0 {
		t.Fatalf("expired artifact must not trigger a download call, got %d", github.downloads)
	}
}

func TestArtifactsDownloadRefusesExistingDestinationUnlessForced(t *testing.T) {
	service := usecase.ArtifactsService{GitHub: fake.New(fake.ScenarioFailing)}
	dest := t.TempDir()
	artifact := model.Artifact{ID: 901, Name: "coverage"}
	if err := os.MkdirAll(filepath.Join(dest, "coverage"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Download(context.Background(), "indrasvat/gh-hound", artifact, dest, false); err == nil {
		t.Fatal("existing destination must error without force")
	}
	if _, err := service.Download(context.Background(), "indrasvat/gh-hound", artifact, dest, true); err != nil {
		t.Fatalf("force download failed: %v", err)
	}
}

func TestArtifactsDownloadBlocksZipSlip(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create("../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("escape")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	github := &countingArtifactGitHub{Adapter: fake.New(fake.ScenarioFailing), zipBytes: buf.Bytes()}
	service := usecase.ArtifactsService{GitHub: github}
	dest := t.TempDir()

	_, err = service.Download(context.Background(), "indrasvat/gh-hound", model.Artifact{ID: 903, Name: "evil"}, dest, false)
	if err == nil {
		t.Fatal("zip-slip entry must be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); !os.IsNotExist(statErr) {
		t.Fatal("zip-slip entry escaped the destination directory")
	}
}

type countingArtifactGitHub struct {
	*fake.Adapter
	downloads int
	zipBytes  []byte
}

func (g *countingArtifactGitHub) DownloadArtifact(ctx context.Context, repo string, artifactID int64) (io.ReadCloser, error) {
	g.downloads++
	if g.zipBytes != nil {
		return io.NopCloser(bytes.NewReader(g.zipBytes)), nil
	}
	return g.Adapter.DownloadArtifact(ctx, repo, artifactID)
}
