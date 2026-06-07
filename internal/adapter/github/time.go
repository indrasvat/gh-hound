package github

import (
	"encoding/json"
	"time"
)

type modelTime struct {
	time.Time
}

func (m *modelTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || string(data) == `""` {
		m.Time = time.Time{}
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return err
	}
	m.Time = parsed
	return nil
}
