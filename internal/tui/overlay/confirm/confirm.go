package confirm

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone    IntentKind = ""
	IntentConfirm IntentKind = "confirm"
	IntentCancel  IntentKind = "cancel"
)

type Intent struct {
	Kind IntentKind
}

type Model struct {
	Message string
	Intent  Intent
}

func New(message string) Model {
	return Model{Message: message}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "y", "Y":
		m.Intent = Intent{Kind: IntentConfirm}
	case "enter", "n", "N", "esc":
		m.Intent = Intent{Kind: IntentCancel}
	}
	return m
}
