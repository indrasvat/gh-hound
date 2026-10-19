package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/config"
	"github.com/indrasvat/gh-hound/internal/theme"
)

func overlayTestApp() App {
	cfg := config.Default()
	cfg.Welcome = false
	return NewApp(Options{Config: cfg})
}

func pressKeys(app App, keys ...string) App {
	for _, key := range keys {
		app, _ = app.Update(KeyMsg{Key: key})
	}
	return app
}

// TestQuestionMarkInHelpClosesIt holds the help footer's promise:
// "? close", not a second help layer under the first.
func TestQuestionMarkInHelpClosesIt(t *testing.T) {
	app := pressKeys(overlayTestApp(), "?")
	if !strings.Contains(app.View(), "? close") {
		t.Fatalf("help footer no longer promises ? close:\n%s", app.View())
	}
	app = pressKeys(app, "?")
	if len(app.overlays) != 0 {
		t.Fatalf("? in help left overlays %v, want help closed", app.overlays)
	}
}

func TestOverlayNeverStacksASecondCopy(t *testing.T) {
	tests := []struct {
		keys string
		want []Overlay
	}{
		{"? ? ?", []Overlay{OverlayHelp}},
		{"? :", []Overlay{OverlayHelp, OverlayPalette}},
		{": ?", []Overlay{OverlayPalette, OverlayHelp}},
		{"? : ?", []Overlay{OverlayHelp}},
		{": ? :", []Overlay{OverlayPalette}},
		{": ? : ? :", []Overlay{OverlayPalette}},
		{"? : ? : ?", []Overlay{OverlayHelp}},
	}
	for _, tt := range tests {
		app := pressKeys(overlayTestApp(), strings.Fields(tt.keys)...)
		if !slices.Equal(app.overlays, tt.want) {
			t.Errorf("%q: overlays = %v, want %v", tt.keys, app.overlays, tt.want)
		}
	}
}

// FuzzOverlayStack drives the overlay keys in any order: no overlay is
// ever open twice, and one esc per open overlay always gets back out.
func FuzzOverlayStack(f *testing.F) {
	for _, seed := range []string{"???", "?:?:?", ":?:?:", "?:x?e:", ":ab?e?"} {
		f.Add([]byte(seed))
	}
	alphabet := map[byte]string{'?': "?", ':': ":", 'e': "esc", 'j': "j", 'x': "x"}
	f.Fuzz(func(t *testing.T, input []byte) {
		app := overlayTestApp()
		for _, b := range input {
			key, ok := alphabet[b]
			if !ok {
				continue
			}
			app, _ = app.Update(KeyMsg{Key: key})
			seen := map[Overlay]bool{}
			for _, overlay := range app.overlays {
				if seen[overlay] {
					t.Fatalf("after %q: %s open twice in %v", input, overlay, app.overlays)
				}
				seen[overlay] = true
			}
		}
		for range len(app.overlays) {
			app, _ = app.Update(KeyMsg{Key: "esc"})
		}
		if len(app.overlays) != 0 {
			t.Fatalf("after %q: one esc per overlay left %v", input, app.overlays)
		}
	})
}

// The palette's footer advertises "theme": it must be a real entry.
func TestPaletteThemeEntryTogglesTheTheme(t *testing.T) {
	app := pressKeys(overlayTestApp(), ":", "t", "h", "e", "m", "e", "enter")
	if app.Theme().Mode != theme.ModeBone || len(app.overlays) != 0 {
		t.Fatalf("theme = %s, overlays = %v; want Bone and the palette closed", app.Theme().Mode, app.overlays)
	}
}
