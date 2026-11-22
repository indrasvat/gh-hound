package render

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

// The schema's verdict and reason enums are the model's: a reason the
// classifier can emit but the schema does not name breaks agents that
// validate the payload.
func TestSchemaNamesEveryVerdictAndReason(t *testing.T) {
	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Properties struct {
			Runs struct {
				Items struct {
					Properties map[string]struct {
						Enum []string `json:"enum"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"runs"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema.json invalid: %v", err)
	}
	props := schema.Properties.Runs.Items.Properties
	verdicts := []string{string(model.VerdictCode), string(model.VerdictNeverRan), string(model.VerdictInfra)}
	reasons := []string{}
	for _, r := range []model.VerdictReason{model.ReasonAwaitingApproval, model.ReasonApprovalMissed, model.ReasonBlockedByPolicy,
		model.ReasonCouldNotStart, model.ReasonNoJobRan, model.ReasonRunnerLost, model.ReasonKilled, model.ReasonDiskFull} {
		reasons = append(reasons, string(r))
	}
	if got := props["verdict"].Enum; !slices.Equal(got, verdicts) {
		t.Fatalf("schema verdict enum = %v, want %v", got, verdicts)
	}
	if got := props["reason"].Enum; !slices.Equal(got, reasons) {
		t.Fatalf("schema reason enum = %v, want %v", got, reasons)
	}
}
