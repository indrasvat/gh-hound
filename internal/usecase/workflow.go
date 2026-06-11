package usecase

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"gopkg.in/yaml.v3"
)

// WorkflowsService lists workflows (state included — it rides the
// existing list fetch, no extra calls) and toggles them on or off.
// Toggle selectors are what the API accepts: a numeric workflow ID or
// the workflow file path. Display names resolve only in the TUI, from
// the list already in hand, so the pipe verbs keep the one-call
// budget.
type WorkflowsService struct {
	GitHub  GitHub
	Limiter *MutationLimiter
}

func (s WorkflowsService) List(ctx context.Context, repo string) ([]model.Workflow, error) {
	return s.GitHub.ListWorkflows(ctx, repo)
}

// Enable wakes a workflow (active again). The success message is the
// one source of hound voice for every surface: TUI toast and pipe.
func (s WorkflowsService) Enable(ctx context.Context, repo, target string) (ActionResult, error) {
	if err := s.prepareToggle(ctx, target); err != nil {
		return ActionResult{}, err
	}
	result, err := s.GitHub.EnableWorkflow(ctx, repo, strings.TrimSpace(target))
	if err != nil {
		return ActionResult{}, err
	}
	result.Message = "back on duty."
	return result, nil
}

// Disable muzzles a workflow (disabled_manually upstream).
func (s WorkflowsService) Disable(ctx context.Context, repo, target string) (ActionResult, error) {
	if err := s.prepareToggle(ctx, target); err != nil {
		return ActionResult{}, err
	}
	result, err := s.GitHub.DisableWorkflow(ctx, repo, strings.TrimSpace(target))
	if err != nil {
		return ActionResult{}, err
	}
	result.Message = "muzzled."
	return result, nil
}

// prepareToggle validates the selector BEFORE the limiter and the
// write: a refusal must never burn an API call or a pacing slot.
func (s WorkflowsService) prepareToggle(ctx context.Context, target string) error {
	if !ValidWorkflowTarget(target) {
		return ActionError{
			Kind:    ActionErrorValidation,
			Field:   "workflow",
			Message: fmt.Sprintf("workflow %q is not a toggle selector — pass the numeric ID or the workflow file path (ci.yml), the two forms the API accepts", strings.TrimSpace(target)),
		}
	}
	if s.Limiter != nil {
		if err := s.Limiter.Wait(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ValidWorkflowTarget reports whether a selector is one the API
// accepts directly: a numeric workflow ID or a .yml/.yaml file
// name/path.
func ValidWorkflowTarget(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		// Zero and negative IDs are never real workflows; refusing
		// here keeps the limiter and the wire untouched.
		return id > 0
	}
	base := path.Base(target)
	return strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")
}

func ParseWorkflowDispatchInputs(raw string) ([]model.WorkflowInput, bool, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, false, err
	}
	doc := documentNode(&root)
	if doc == nil || doc.Kind != yaml.MappingNode {
		return nil, false, nil
	}
	on := mappingValue(doc, "on")
	if on == nil {
		return nil, false, nil
	}
	dispatch := dispatchNode(on)
	if dispatch == nil {
		return nil, false, nil
	}
	if dispatch.Kind != yaml.MappingNode {
		return nil, true, nil
	}
	inputsNode := mappingValue(dispatch, "inputs")
	if inputsNode == nil || inputsNode.Kind != yaml.MappingNode {
		return nil, true, nil
	}
	inputs := make([]model.WorkflowInput, 0, len(inputsNode.Content)/2)
	for i := 0; i+1 < len(inputsNode.Content); i += 2 {
		name := strings.TrimSpace(inputsNode.Content[i].Value)
		if name == "" {
			continue
		}
		input, err := parseWorkflowInput(name, inputsNode.Content[i+1])
		if err != nil {
			return nil, false, err
		}
		inputs = append(inputs, input)
	}
	return inputs, true, nil
}

func documentNode(root *yaml.Node) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return root.Content[0]
	}
	return root
}

func dispatchNode(on *yaml.Node) *yaml.Node {
	switch on.Kind {
	case yaml.ScalarNode:
		if on.Value == "workflow_dispatch" {
			return on
		}
	case yaml.SequenceNode:
		for _, item := range on.Content {
			if item.Value == "workflow_dispatch" {
				return item
			}
		}
	case yaml.MappingNode:
		return mappingValue(on, "workflow_dispatch")
	}
	return nil
}

func parseWorkflowInput(name string, node *yaml.Node) (model.WorkflowInput, error) {
	input := model.WorkflowInput{Name: name, Type: "string"}
	if node == nil || node.Kind != yaml.MappingNode {
		return input, nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]
		switch key {
		case "description":
			input.Description = scalarString(value)
		case "required":
			input.Required = scalarBool(value)
		case "type":
			input.Type = normalizeInputType(scalarString(value))
		case "default":
			input.Default = scalarString(value)
		case "options":
			options, err := scalarStringSlice(value)
			if err != nil {
				return model.WorkflowInput{}, fmt.Errorf("workflow_dispatch input %q options: %w", name, err)
			}
			input.Options = options
		}
	}
	return input, nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func scalarString(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func scalarBool(node *yaml.Node) bool {
	value := strings.ToLower(scalarString(node))
	return value == "true" || value == "yes" || value == "on"
}

func scalarStringSlice(node *yaml.Node) ([]string, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("expected sequence, got %s", node.ShortTag())
	}
	out := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if value := scalarString(item); value != "" {
			out = append(out, value)
		}
	}
	return out, nil
}

func normalizeInputType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "boolean", "bool":
		return "boolean"
	case "choice", "select":
		return "choice"
	case "number":
		return "number"
	case "environment":
		return "environment"
	case "string", "":
		return "string"
	default:
		return value
	}
}
