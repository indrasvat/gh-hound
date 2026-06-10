package repository

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

type GitFunc func(context.Context, ...string) (string, error)

type Detector struct {
	WorkDir   string
	LookupEnv func(string) (string, bool)
	Git       GitFunc
}

type notFoundError struct {
	reason string
}

func (e notFoundError) Error() string {
	return e.reason
}

func IsNotFound(err error) bool {
	var target notFoundError
	return errors.As(err, &target)
}

func (d Detector) Current(ctx context.Context) (usecase.RepositoryContext, error) {
	lookup := d.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	git := d.Git
	if git == nil {
		git = d.runGit
	}

	repo := firstEnv(lookup, "GH_REPO", "HOUND_REPO")
	if repo == "" {
		remote, err := git(ctx, "remote", "get-url", "origin")
		if err != nil {
			return usecase.RepositoryContext{}, notFoundError{reason: "not a git repo / no resolvable remote"}
		}
		parsed, ok := parseGitHubRemote(strings.TrimSpace(remote))
		if !ok {
			return usecase.RepositoryContext{}, notFoundError{reason: "no GitHub remote found"}
		}
		repo = parsed
	}

	branch, _ := git(ctx, "branch", "--show-current")
	headSHA, _ := git(ctx, "rev-parse", "HEAD")
	actor, _ := git(ctx, "config", "user.name")
	repoCtx := usecase.RepositoryContext{
		Repo:    strings.TrimSpace(repo),
		Branch:  strings.TrimSpace(branch),
		HeadSHA: strings.TrimSpace(headSHA),
		Actor:   strings.TrimSpace(actor),
	}
	repoCtx.Detached = repoCtx.Branch == ""
	return repoCtx, nil
}

func (d Detector) runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if d.WorkDir != "" {
		cmd.Dir = d.WorkDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func parseGitHubRemote(raw string) (string, bool) {
	raw = strings.TrimSuffix(strings.TrimSpace(raw), ".git")
	if raw == "" {
		return "", false
	}
	if after, ok := strings.CutPrefix(raw, "git@github.com:"); ok {
		return cleanRepo(after)
	}
	if after, ok := strings.CutPrefix(raw, "ssh://git@github.com/"); ok {
		return cleanRepo(after)
	}
	parsed, err := url.Parse(raw)
	if err == nil && strings.EqualFold(parsed.Host, "github.com") {
		return cleanRepo(strings.TrimPrefix(parsed.Path, "/"))
	}
	return "", false
}

func cleanRepo(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

func firstEnv(lookup func(string) (string, bool), keys ...string) string {
	for _, key := range keys {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
