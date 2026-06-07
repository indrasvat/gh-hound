package dispatch

import (
	"strings"

	"github.com/indrasvat/gh-hound/internal/tui/keys"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type InputType string

const (
	InputText   InputType = "text"
	InputBool   InputType = "bool"
	InputSelect InputType = "select"
)

type Workflow struct {
	Name   string
	ID     string
	Ref    string
	Inputs []Input
}

type Input struct {
	Name     string
	Required bool
	Type     InputType
	Options  []string
}

type Field struct {
	Input
	Value string
	Index int
}

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone   IntentKind = ""
	IntentSubmit IntentKind = "submit"
	IntentCancel IntentKind = "cancel"
)

type Intent struct {
	Kind    IntentKind
	Request usecase.DispatchRequest
}

type Model struct {
	Workflow Workflow
	Fields   []Field
	Focused  int
	Intent   Intent
	Error    string
}

func NewModel(workflow Workflow) Model {
	fields := make([]Field, len(workflow.Inputs))
	for i, input := range workflow.Inputs {
		fields[i] = Field{Input: input}
		if len(input.Options) > 0 {
			fields[i].Value = input.Options[0]
		}
	}
	return Model{Workflow: workflow, Fields: fields}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	m.Error = ""
	if len(m.Fields) == 0 {
		return m
	}
	field := &m.Fields[m.Focused]
	switch msg.Key {
	case "tab":
		m.Focused = (m.Focused + 1) % len(m.Fields)
	case "shift+tab":
		m.Focused = (m.Focused + len(m.Fields) - 1) % len(m.Fields)
	case "right", "space":
		if len(field.Options) > 0 {
			field.Index = (field.Index + 1) % len(field.Options)
			field.Value = field.Options[field.Index]
		}
	case "left":
		if len(field.Options) > 0 {
			field.Index = (field.Index + len(field.Options) - 1) % len(field.Options)
			field.Value = field.Options[field.Index]
		}
	case "backspace":
		if field.Type == InputText && len(field.Value) > 0 {
			field.Value = field.Value[:len(field.Value)-1]
		}
	case "enter":
		m.submit()
	case "esc":
		m.Intent = Intent{Kind: IntentCancel}
	default:
		if field.Type == InputText && len([]rune(msg.Key)) == 1 {
			field.Value += msg.Key
		}
	}
	return m
}

func (m *Model) submit() {
	inputs := map[string]string{}
	required := []string{}
	for _, field := range m.Fields {
		inputs[field.Name] = field.Value
		if field.Required {
			required = append(required, field.Name)
		}
	}
	request := usecase.DispatchRequest{Ref: m.Workflow.Ref, Inputs: inputs, RequiredInputs: required}
	for _, requiredInput := range required {
		if inputs[requiredInput] == "" {
			m.Error = requiredInput + " is required"
			return
		}
	}
	m.Intent = Intent{Kind: IntentSubmit, Request: request}
}

func View(m Model, width int) string {
	lines := []string{"dispatch · " + m.Workflow.Name, "ref " + m.Workflow.Ref + " ▾"}
	for i, field := range m.Fields {
		prefix := " "
		if i == m.Focused {
			prefix = "▌"
		}
		lines = append(lines, fit(prefix+field.Name+" "+field.Value, width))
	}
	lines = append(lines, "POST …/workflows/"+m.Workflow.ID+"/dispatches", keys.FooterForScreen(keys.ScreenDispatch))
	return strings.Join(lines, "\n")
}

func fit(value string, width int) string {
	if width <= 0 {
		width = 80
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}
