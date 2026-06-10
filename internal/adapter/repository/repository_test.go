package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDetectorResolvesEnvBeforeGitRemote(t *testing.T) {
	detector := Detector{
		LookupEnv: func(key string) (string, bool) {
			if key == "GH_REPO" {
				return "indrasvat/env-repo", true
			}
			return "", false
		},
		Git: fakeGit(map[string]string{
			"branch --show-current": "main\n",
			"rev-parse HEAD":        "a1b2c3d4\n",
		}),
	}

	got, err := detector.Current(context.Background())
	if err != nil {
		t.Fatalf("Current returned error: %v", err)
	}
	if got.Repo != "indrasvat/env-repo" || got.Branch != "main" || got.HeadSHA != "a1b2c3d4" {
		t.Fatalf("context = %#v", got)
	}
}

func TestDetectorParsesGitHubRemoteAndDetachedHead(t *testing.T) {
	detector := Detector{
		Git: fakeGit(map[string]string{
			"remote get-url origin": "git@github.com:indrasvat/gh-hound.git\n",
			"branch --show-current": "",
			"rev-parse HEAD":        "a1b2c3d4\n",
		}),
	}

	got, err := detector.Current(context.Background())
	if err != nil {
		t.Fatalf("Current returned error: %v", err)
	}
	if got.Repo != "indrasvat/gh-hound" || !got.Detached || got.Branch != "" {
		t.Fatalf("context = %#v", got)
	}
}

func TestDetectorErrorsWhenRepoCannotResolve(t *testing.T) {
	detector := Detector{Git: func(context.Context, ...string) (string, error) {
		return "", errors.New("not a git repo")
	}}

	_, err := detector.Current(context.Background())
	if !IsNotFound(err) {
		t.Fatalf("err = %#v, want repository not found", err)
	}
}

func fakeGit(outputs map[string]string) func(context.Context, ...string) (string, error) {
	return func(ctx context.Context, args ...string) (string, error) {
		key := strings.Join(args, " ")
		out, ok := outputs[key]
		if !ok {
			return "", errors.New("unexpected git command: " + key)
		}
		return out, nil
	}
}
