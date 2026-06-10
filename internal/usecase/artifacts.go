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

// DestinationExistsError reports a refused extraction target. Callers
// add their own recovery hint: the CLI suggests --force, the TUI does
// not advertise CLI-only flags.
type DestinationExistsError struct {
	Path string
}

func (e DestinationExistsError) Error() string {
	return fmt.Sprintf("destination %s already exists", e.Path)
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
	// The artifact name is API-provided input and becomes the extraction
	// root (and a RemoveAll target under --force): require a single
	// local path element so a hostile name can never escape destDir.
	if artifact.Name == "" || filepath.Base(artifact.Name) != artifact.Name || !filepath.IsLocal(artifact.Name) {
		return DownloadResult{}, fmt.Errorf("artifact name %q is not a safe directory name", artifact.Name)
	}
	target := filepath.Join(destDir, artifact.Name)
	if _, err := os.Stat(target); err == nil {
		if !force {
			return DownloadResult{}, DestinationExistsError{Path: target}
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
	count, err := extractZip(reader, target, extractionBudget(size))
	if err != nil {
		// Never leave a half-extracted destination behind.
		_ = os.RemoveAll(target)
		return DownloadResult{}, err
	}
	return DownloadResult{Path: target, FileCount: count}, nil
}

const (
	// extractionRatioCap bounds decompressed output relative to the
	// archive size to stop zip bombs from exhausting the disk; the
	// floor keeps legitimately compressible artifacts (logs, JSON)
	// unaffected.
	extractionRatioCap  = 100
	extractionBudgetMin = int64(1) << 30
)

func extractionBudget(archiveSize int64) int64 {
	budget := archiveSize * extractionRatioCap
	if budget < extractionBudgetMin {
		return extractionBudgetMin
	}
	return budget
}

func extractZip(reader *zip.Reader, target string, budget int64) (int, error) {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return 0, err
	}
	count := 0
	remaining := budget
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
		// Symlink entries are deliberately materialized as regular
		// files holding the target string: reconstructing symlinks
		// from untrusted archives reopens zip-slip via link targets.
		written, err := extractZipFile(file, dest, remaining)
		remaining -= written
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func extractZipFile(file *zip.File, dest string, budget int64) (int64, error) {
	src, err := file.Open()
	if err != nil {
		return 0, err
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
		return 0, err
	}
	written, err := io.Copy(out, io.LimitReader(src, budget+1))
	if err != nil {
		_ = out.Close()
		return written, err
	}
	if written > budget {
		_ = out.Close()
		return written, fmt.Errorf("artifact expands past the %d-byte safety budget; refusing to extract further", budget)
	}
	return written, out.Close()
}
