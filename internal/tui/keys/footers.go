package keys

import "strings"

type Screen string

const (
	ScreenWelcome  Screen = "welcome"
	ScreenAllGreen Screen = "all_green"
	ScreenRunsList Screen = "runs_list"
	ScreenDetail   Screen = "detail"
	ScreenFailure  Screen = "failure"
	ScreenWatch    Screen = "watch"
	ScreenLog      Screen = "log"
	ScreenDispatch Screen = "dispatch"
	ScreenPalette  Screen = "palette"
	ScreenHelp     Screen = "help"
	ScreenToasts   Screen = "toasts"
)

var footerByScreen = map[Screen][]Binding{
	ScreenWelcome: {
		{Key: "enter", Display: "⏎", Action: "continue", Help: "continue", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "help", ShowInFooter: true},
		{Key: "q", Action: "quit", Help: "quit", ShowInFooter: true},
	},
	ScreenAllGreen: {
		{Key: "w", Action: "watch_next_push", Help: "watch next push", ShowInFooter: true},
		{Key: "D", Action: "dispatch", Help: "dispatch", ShowInFooter: true},
		{Key: "/", Action: "filter", Help: "filter", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "help", ShowInFooter: true},
	},
	ScreenRunsList: {
		{Key: "enter", Display: "⏎", Action: "open", Help: "open", ShowInFooter: true},
		{Key: "r", Display: "↻", Action: "rerun", Help: "rerun", ShowInFooter: true},
		{Key: "x", Display: "✗", Action: "cancel", Help: "cancel", ShowInFooter: true},
		{Key: "l", Action: "logs", Help: "logs", ShowInFooter: true},
		{Key: "w", Action: "watch", Help: "watch", ShowInFooter: true},
		{Key: "/", Action: "filter", Help: "filter", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "help", ShowInFooter: true},
	},
	ScreenDetail: {
		{Key: "enter", Display: "⏎", Action: "expand", Help: "expand", ShowInFooter: true},
		{Key: "r", Display: "↻", Action: "rerun_job", Help: "rerun job", ShowInFooter: true},
		{Key: "R", Action: "rerun_failed", Help: "rerun failed", ShowInFooter: true},
		{Key: "x", Display: "✗", Action: "cancel", Help: "cancel", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
		{Key: "?", Action: "help", Help: "", ShowInFooter: true},
	},
	ScreenFailure: {
		{Key: "R", Display: "↻", Action: "rerun_failed", Help: "rerun failed", ShowInFooter: true},
		{Key: "r", Action: "rerun_job", Help: "rerun job", ShowInFooter: true},
		{Key: "l", Action: "full_log", Help: "full log", ShowInFooter: true},
		{Key: "o", Action: "browser", Help: "browser", ShowInFooter: true},
		{Key: "y", Action: "copy_excerpt", Help: "copy excerpt", ShowInFooter: true},
	},
	ScreenWatch: {
		{Key: "x", Display: "✗", Action: "cancel", Help: "cancel", ShowInFooter: true},
		{Key: "f", Action: "follow", Help: "follow", ShowInFooter: true},
		{Key: "d", Action: "debug", Help: "debug", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "detach", Help: "detach", ShowInFooter: true},
	},
	ScreenLog: {
		{Key: "j/k", Action: "scroll", Help: "scroll", ShowInFooter: true},
		{Key: "g/G", Action: "top_bottom", Help: "ends", ShowInFooter: true},
		{Key: "/", Action: "search", Help: "search", ShowInFooter: true},
		{Key: "n/N", Action: "match", Help: "match", ShowInFooter: true},
		{Key: "z/Z", Action: "fold", Help: "fold", ShowInFooter: true},
		{Key: "w", Action: "wrap", Help: "wrap", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "back", Help: "back", ShowInFooter: true},
	},
	ScreenDispatch: {
		{Key: "enter", Display: "⏎", Action: "run", Help: "run", ShowInFooter: true},
		{Key: "tab", Display: "⇥", Action: "next", Help: "next", ShowInFooter: true},
		{Key: "esc", Display: "⎋", Action: "cancel", Help: "cancel", ShowInFooter: true},
	},
	ScreenPalette: {
		{Key: "workflows", Action: "workflows", Help: "", ShowInFooter: true},
		{Key: "watch", Action: "watch", Help: "", ShowInFooter: true},
		{Key: "diff (v2)", Action: "diff_v2", Help: "", ShowInFooter: true},
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

func FooterForScreen(screen Screen) string {
	return footerString(footerByScreen[screen])
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
