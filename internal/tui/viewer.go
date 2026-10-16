package tui

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

// viewerLookupTimeout bounds the shared /user call; it runs detached
// from any one load so a superseded load cannot strand it.
const viewerLookupTimeout = 15 * time.Second

// viewerCache resolves the authenticated login once per session and
// shares it: the header identity, actor:@me loads and polls all read
// the same lookup. A success is kept for the session; a failure is kept
// until the user re-applies a query (Retry), so polls never repeat it.
// Pointer-held so the lookup survives App value copies.
type viewerCache struct {
	resolve func(context.Context) (string, error)

	mu      sync.Mutex
	login   string
	err     error
	pending chan struct{}
}

func newViewerCache(resolve func(context.Context) (string, error), known string) *viewerCache {
	return &viewerCache{resolve: resolve, login: known}
}

// Start begins the lookup in the background unless it is known,
// failed, or already running. Never blocks.
func (c *viewerCache) Start() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startLocked()
}

func (c *viewerCache) startLocked() chan struct{} {
	if c.login != "" || c.err != nil {
		return nil
	}
	if c.pending != nil {
		return c.pending
	}
	if c.resolve == nil {
		c.err = usecase.ViewerError{}
		return nil
	}
	done := make(chan struct{})
	c.pending = done
	resolve := c.resolve
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), viewerLookupTimeout)
		defer cancel()
		login, err := resolve(ctx)
		if err == nil && login == "" {
			err = errors.New("GitHub returned no login")
		}
		c.mu.Lock()
		if err != nil {
			c.err = usecase.ViewerError{Err: err}
		} else {
			c.login = login
		}
		c.pending = nil
		c.mu.Unlock()
		close(done)
	}()
	return done
}

// Resolve returns the login, waiting on the shared lookup. Only the
// caller's ctx can cut the wait short; the lookup itself carries on.
func (c *viewerCache) Resolve(ctx context.Context) (string, error) {
	if c == nil {
		return "", usecase.ViewerError{}
	}
	c.mu.Lock()
	done := c.startLocked()
	login, err := c.login, c.err
	c.mu.Unlock()
	if done == nil {
		return login, err
	}
	select {
	case <-done:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.login, c.err
}

// Login is the render-time view: "" until the lookup lands.
func (c *viewerCache) Login() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.login
}

// Pending reports an in-flight lookup, so the loop ticks fast enough
// to paint its result promptly.
func (c *viewerCache) Pending() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending != nil
}

// Retry forgets a cached failure; the next Resolve asks GitHub again.
func (c *viewerCache) Retry() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = nil
}
