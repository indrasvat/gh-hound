package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestAuthenticatedLoginReadsUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"login":"BagToad","id":1}`))
	}))
	defer server.Close()
	login, err := NewClient(server.URL, server.Client()).AuthenticatedLogin(context.Background())
	if err != nil || login != "BagToad" {
		t.Fatalf("AuthenticatedLogin = %q, %v", login, err)
	}
}

func TestAuthenticatedLoginTypesAuthFailures(t *testing.T) {
	for status, kind := range map[int]usecase.APIErrorKind{
		http.StatusUnauthorized: usecase.APIErrorAuth,
		http.StatusForbidden:    usecase.APIErrorPermission,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"Requires authentication"}`))
		}))
		_, err := NewClient(server.URL, server.Client()).AuthenticatedLogin(context.Background())
		server.Close()
		var apiErr usecase.APIError
		if !errors.As(err, &apiErr) || apiErr.Kind != kind {
			t.Fatalf("status %d: error = %#v, want kind %s", status, err, kind)
		}
	}
}

func TestAuthenticatedLoginRejectsEmptyLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	if _, err := NewClient(server.URL, server.Client()).AuthenticatedLogin(context.Background()); err == nil {
		t.Fatal("empty login must be an error, not an empty actor filter")
	}
}
