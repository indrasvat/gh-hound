package usecase

import (
	"fmt"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"gopkg.in/yaml.v3"
)

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
