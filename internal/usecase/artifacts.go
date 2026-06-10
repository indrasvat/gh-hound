package usecase

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/indrasvat/gh-hound/internal/model"
)

// ArtifactsService lists a run's artifacts and downloads one with
// gh-run-download-style extraction into <dest>/<artifact-name>/.
type ArtifactsService struct {
	GitHub GitHub
}

// ArtifactExpiredError is returned before any network call when the
// artifact's retention window has lapsed.
type ArtifactExpiredError struct {
	Name string
}

func (e ArtifactExpiredError) Error() string {
	return fmt.Sprintf("artifact %q has expired and can no longer be downloaded", e.Name)
}

type DownloadResult struct {
	Path      string
	FileCount int
}

func (s ArtifactsService) List(ctx context.Context, repo string, runID int64) ([]model.Artifact, error) {
	return s.GitHub.ListArtifacts(ctx, repo, runID)
}

func (s ArtifactsService) Download(ctx context.Context, repo string, artifact model.Artifact, destDir string, force bool) (DownloadResult, error) {
	if artifact.Expired {
		return DownloadResult{}, ArtifactExpiredError{Name: artifact.Name}
	}
	target := filepath.Join(destDir, artifact.Name)
	if _, err := os.Stat(target); err == nil {
		if !force {
			return DownloadResult{}, fmt.Errorf("destination %s already exists; pass --force to overwrite", target)
		}
		if err := os.RemoveAll(target); err != nil {
			return DownloadResult{}, fmt.Errorf("clear destination: %w", err)
		}
	}

	body, err := s.GitHub.DownloadArtifact(ctx, repo, artifact.ID)
	if err != nil {
		return DownloadResult{}, err
	}
	defer func() {
		_ = body.Close()
	}()

	// Stream to a temp file: artifacts can be large and the zip reader
	// needs random access, so never buffer the archive in memory.
	temp, err := os.CreateTemp("", "gh-hound-artifact-*.zip")
	if err != nil {
		return DownloadResult{}, err
	}
	defer func() {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
	}()
	size, err := io.Copy(temp, body)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("download artifact %q: %w", artifact.Name, err)
	}

	reader, err := zip.NewReader(temp, size)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("artifact %q is not a valid zip archive: %w", artifact.Name, err)
	}
	count, err := extractZip(reader, target)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{Path: target, FileCount: count}, nil
}

func extractZip(reader *zip.Reader, target string) (int, error) {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return 0, err
	}
	count := 0
	for _, file := range reader.File {
		// Zip-slip guard: entry paths must stay inside the target dir.
		if !filepath.IsLocal(filepath.FromSlash(file.Name)) {
			return count, fmt.Errorf("artifact entry %q escapes the destination directory", file.Name)
		}
		dest := filepath.Join(target, filepath.FromSlash(file.Name))
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return count, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return count, err
		}
		if err := extractZipFile(file, dest); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func extractZipFile(file *zip.File, dest string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer func() {
		_ = src.Close()
	}()
	mode := file.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
