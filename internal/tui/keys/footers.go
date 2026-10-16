package keys

import (
	"strings"
	"unicode/utf8"
)

type Screen string

const (
	ScreenWelcome    Screen = "welcome"
	ScreenAllGreen   Screen = "all_green"
	ScreenRunsList   Screen = "runs_list"
	ScreenRunsFilter Screen = "runs_filter"
	ScreenDetail     Screen = "detail"
	ScreenFailure    Screen = "failure"
	ScreenWatch      Screen = "watch"
	ScreenWatchBoard Screen = "watch_board"
	ScreenLog        Screen = "log"
	ScreenDispatch   Screen = "dispatch"
	ScreenDiff       Screen = "diff"
	ScreenFlakes     Screen = "flakes"
	ScreenCaches     Screen = "caches"
	ScreenQueue      Screen = "queue"
	ScreenWorkflows  Screen = "workflows"
	ScreenPalette    Screen = "palette"
	ScreenHelp       Screen = "help"
	ScreenToasts     Screen = "toasts"
)

var footerByScreen = map[Screen][]Binding{
	ScreenWelcome: {
		{Key: "enter", Display: "⏎", Action: "continue", Help: "continue", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "help", ShowInFooter: true},
		{Key: "q", Action: "quit", Help: "quit", ShowInFooter: true},
	},
	ScreenAllGreen: {
		{Key: "j/k", Action: "move", Help: "move", ShowInFooter: true},
		{Key: "s", Action: "scope", Help: "scope", ShowInFooter: true},
		{Key: "enter", Display: "⏎", Action: "open", Help: "open", ShowInFooter: true},
		{Key: "w", Action: "watch_next_push", Help: "watch", ShowInFooter: true},
		{Key: "D", Action: "dispatch", Help: "dispatch", ShowInFooter: true},
		{Key: "/", Action: "filter", Help: "filter", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "help", ShowInFooter: true},
	},
	ScreenRunsList: {
		{Key: "j/k", Action: "move", Help: "", ShowInFooter: true},
		{Key: "s", Action: "scope", Help: "scope", ShowInFooter: true},
		{Key: "enter", Display: "⏎", Action: "open", Help: "open", ShowInFooter: true},
		{Key: "r", Display: "↻", Action: "rerun", Help: "rerun", ShowInFooter: true},
		{Key: "x", Display: "✗", Action: "cancel", Help: "cancel", ShowInFooter: true},
		{Key: "l", Action: "logs", Help: "log", ShowInFooter: true},
		{Key: "w", Action: "watch", Help: "watch", ShowInFooter: true},
		{Key: "f", Action: "status_cycle", Help: "status", ShowInFooter: true},
		// The summary line advertises A contextually when a gated run
		// is on screen; the static footer stays truthful for lists
		// without waiting runs (artifacts precedent on detail).
		{Key: "A", Action: "approvals", Help: "approvals (waiting runs)", ShowInFooter: false},
		{Key: "/", Action: "filter", Help: "filter", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "", ShowInFooter: true},
	},
	// While the / input is open every printable key types; only these
	// edit keys act.
	ScreenRunsFilter: {
		{Key: "enter", Display: "⏎", Action: "apply", Help: "apply", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "cancel", Help: "cancel", ShowInFooter: true},
		{Key: "ctrl+u", Action: "clear", Help: "clear", ShowInFooter: true},
	},
	ScreenDetail: {
		{Key: "enter", Display: "⏎", Action: "expand", Help: "expand", ShowInFooter: true},
		{Key: "r", Display: "↻", Action: "rerun_job", Help: "rerun job", ShowInFooter: true},
		{Key: "R", Action: "rerun_failed", Help: "rerun failed", ShowInFooter: true},
		{Key: "x", Display: "✗", Action: "cancel", Help: "cancel", ShowInFooter: true},
		// In-pane hint line advertises these contextually; the static
		// footer stays truthful for runs without artifacts.
		{Key: "a", Action: "artifacts", Help: "artifacts (when present)", ShowInFooter: false},
		{Key: "d", Action: "download", Help: "download artifact", ShowInFooter: false},
		{Key: "o", Action: "open_artifact_dir", Help: "open artifact folder / browser", ShowInFooter: false},
		{Key: "y", Action: "copy_artifact_path", Help: "copy artifact path / run URL", ShowInFooter: false},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "", ShowInFooter: true},
	},
	ScreenFailure: {
		{Key: "R", Display: "↻", Action: "rerun_failed", Help: "rerun failed", ShowInFooter: true},
		{Key: "r", Action: "rerun_job", Help: "rerun job", ShowInFooter: true},
		{Key: "l", Action: "full_log", Help: "full log", ShowInFooter: true},
		{Key: "o", Action: "browser", Help: "browser", ShowInFooter: true},
		{Key: "y", Action: "copy_excerpt", Help: "copy excerpt", ShowInFooter: true},
		// The in-pane hint advertises these contextually once the
		// flake panel lands; the static footer stays truthful for
		// failures without a scent (artifacts precedent on detail).
		{Key: "tab", Display: "⇥", Action: "flake_panel", Help: "flake panel (when scented)", ShowInFooter: false},
		{Key: "j/k", Action: "scroll", Help: "scroll excerpt / evidence", ShowInFooter: false},
	},
	ScreenWatch: {
		{Key: "x", Display: "✗", Action: "cancel", Help: "cancel", ShowInFooter: true},
		{Key: "f", Action: "follow", Help: "follow", ShowInFooter: true},
		{Key: "d", Action: "debug", Help: "debug", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "detach", Help: "detach", ShowInFooter: true},
	},
	ScreenWatchBoard: {
		{Key: "j/k", Action: "move", Help: "move", ShowInFooter: true},
		{Key: "enter", Display: "⏎", Action: "open", Help: "drill in", ShowInFooter: true},
		{Key: "f", Action: "follow_worst", Help: "follow worst", ShowInFooter: true},
		{Key: "x", Display: "✗", Action: "cancel", Help: "cancel", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
	},
	ScreenLog: {
		{Key: "j/k", Action: "scroll", Help: "scroll", ShowInFooter: true, Keep: 4},
		{Key: "g/G", Action: "top_bottom", Help: "ends", ShowInFooter: true, Keep: 1},
		{Key: "/", Action: "search", Help: "search", ShowInFooter: true, Keep: 4},
		{Key: "t", Action: "time_jump", Help: "time", ShowInFooter: true, Keep: 2},
		{Key: "n/N", Action: "match", Help: "match", ShowInFooter: true, Keep: 2},
		{Key: "z/Z", Action: "fold", Help: "fold", ShowInFooter: true, Keep: 3},
		{Key: "c", Action: "compact", Help: "compact", ShowInFooter: true, Keep: 4},
		{Key: "w", Action: "wrap", Help: "wrap", ShowInFooter: true, Keep: 1},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true, Keep: KeepAlways},
	},
	ScreenDispatch: {
		{Key: "enter", Display: "⏎", Action: "run", Help: "run", ShowInFooter: true},
		{Key: "tab", Display: "⇥", Action: "next", Help: "next", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "cancel", Help: "cancel", ShowInFooter: true},
	},
	ScreenDiff: {
		{Key: "j/k", Action: "move", Help: "move", ShowInFooter: true},
		{Key: "enter", Display: "⏎", Action: "first_bad", Help: "first bad", ShowInFooter: true},
		{Key: "o", Action: "compare", Help: "compare", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
	},
	ScreenFlakes: {
		{Key: "j/k", Action: "move", Help: "move", ShowInFooter: true},
		{Key: "enter", Display: "⏎", Action: "open_run", Help: "open run", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
	},
	ScreenCaches: {
		{Key: "j/k", Action: "move", Help: "move", ShowInFooter: true},
		{Key: "s", Action: "sort", Help: "sort", ShowInFooter: true},
		{Key: "/", Action: "filter", Help: "filter", ShowInFooter: true},
		{Key: "d", Action: "dig_up", Help: "dig up", ShowInFooter: true},
		{Key: "D", Action: "dig_up_key", Help: "dig up key", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "", ShowInFooter: true},
	},
	ScreenQueue: {
		{Key: "j/k", Action: "move", Help: "move", ShowInFooter: true},
		{Key: "enter", Display: "⏎", Action: "open_run", Help: "open run", ShowInFooter: true},
		{Key: "y", Action: "copy_json", Help: "copy JSON", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "", ShowInFooter: true},
	},
	ScreenWorkflows: {
		{Key: "j/k", Action: "move", Help: "move", ShowInFooter: true},
		{Key: "e", Action: "toggle", Help: "wake/muzzle", ShowInFooter: true},
		{Key: "o", Action: "browser", Help: "browser", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
	},
	ScreenPalette: {
		{Key: "workflows", Action: "workflows", Help: "", ShowInFooter: true},
		{Key: "watch", Action: "watch", Help: "", ShowInFooter: true},
		{Key: "diff", Action: "diff", Help: "", ShowInFooter: true},
		{Key: "theme", Action: "theme", Help: "", ShowInFooter: true},
	},
	ScreenHelp: {
		{Key: ":", Action: "palette", Help: "palette", ShowInFooter: true},
		{Key: "?", Action: "close", Help: "close", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "close", Help: "close", ShowInFooter: true},
	},
	ScreenToasts: {
		{Key: "esc", Display: "⎋", Action: "dismiss", Help: "dismiss", ShowInFooter: true},
		{Key: "g", Action: "dismiss_all", Help: "dismiss all", ShowInFooter: true},
		{Key: "r", Action: "retry", Help: "retry", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "help", ShowInFooter: true},
	},
}

// RunsFilterSyntax is the / grammar the runs help teaches; the in-list
// hint row shows concrete examples. Kept to three lines at 80x24 so
// the help overlay still fits with its legend.
var RunsFilterSyntax = []string{
	"actor:@me", "actor:<login>", "status:", "branch:", "event:", "sha:",
	"other text searches loaded rows",
	"f · s keep the rest",
	": My runs / All actors",
}

func FooterForScreen(screen Screen) string {
	return footerString(footerByScreen[screen])
}

// FooterForScreenWidth drops the lowest-Keep hints (rightmost first) until
// the footer fits width; screens without Keep values are returned whole.
func FooterForScreenWidth(screen Screen, width int) string {
	var shown []Binding
	prioritized := false
	for _, b := range footerByScreen[screen] {
		if b.ShowInFooter {
			shown = append(shown, b)
			prioritized = prioritized || b.Keep > 0
		}
	}
	footer := footerString(shown)
	for prioritized && utf8.RuneCountInString(footer) > width {
		drop := -1
		for i, b := range shown {
			if b.Keep < KeepAlways && (drop < 0 || b.Keep <= shown[drop].Keep) {
				drop = i
			}
		}
		if drop < 0 {
			break
		}
		shown = append(shown[:drop], shown[drop+1:]...)
		footer = footerString(shown)
	}
	return footer
}

func FooterLayer(screen Screen) Layer {
	return Layer{
		Name:     string(screen),
		Bindings: append([]Binding(nil), footerByScreen[screen]...),
	}
}

func footerString(bindings []Binding) string {
	items := ShortHelp(Layer{Bindings: bindings})
	var out strings.Builder
	for i, item := range items {
		if i > 0 {
			out.WriteString(" · ")
		}
		out.WriteString(item)
	}
	return out.String()
}
