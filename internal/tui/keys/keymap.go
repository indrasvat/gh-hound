package keys

import (
	"fmt"
	"unicode/utf8"
)

const ActionInsertText = "insert_text"

type Binding struct {
	Key          string
	Action       string
	Help         string
	Group        string
	ShowInFooter bool
}

type Layer struct {
	Name     string
	Bindings []Binding
}

type ResolveInput struct {
	Key       string
	InputMode bool
	Overlay   Layer
	Screen    Layer
	Global    Layer
}

type Result struct {
	Action  string
	Binding Binding
	Handled bool
}

func ValidateLayer(layer Layer) error {
	seen := map[string]string{}
	for _, binding := range layer.Bindings {
		if binding.Key == "" {
			return fmt.Errorf("layer %s has empty key for action %s", layer.Name, binding.Action)
		}
		if previous, ok := seen[binding.Key]; ok {
			return fmt.Errorf("layer %s key %q binds both %s and %s", layer.Name, binding.Key, previous, binding.Action)
		}
		seen[binding.Key] = binding.Action
	}
	return nil
}

func Resolve(input ResolveInput) Result {
	if input.InputMode && printable(input.Key) {
		return Result{Action: ActionInsertText, Handled: true}
	}
	for _, layer := range []Layer{input.Overlay, input.Screen, input.Global} {
		for _, binding := range layer.Bindings {
			if binding.Key == input.Key {
				return Result{Action: binding.Action, Binding: binding, Handled: true}
			}
		}
	}
	return Result{}
}

func ShortHelp(layer Layer) []string {
	items := make([]string, 0, len(layer.Bindings))
	for _, binding := range layer.Bindings {
		if binding.ShowInFooter {
			if binding.Help == "" {
				items = append(items, binding.Key)
				continue
			}
			items = append(items, binding.Key+" "+binding.Help)
		}
	}
	return items
}

func printable(key string) bool {
	if key == "" {
		return false
	}
	if key == "esc" || key == "enter" || key == "tab" || key == "shift+tab" || key == "ctrl+c" {
		return false
	}
	return utf8.RuneCountInString(key) == 1
}
