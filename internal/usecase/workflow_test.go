package usecase

import (
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

func TestParseWorkflowDispatchInputs(t *testing.T) {
	raw := `
name: Release
on:
  workflow_dispatch:
    inputs:
      version:
        description: Version to release
        required: true
        type: string
      prerelease:
        required: false
        type: boolean
        default: "false"
      channel:
        required: true
        type: choice
        default: beta
        options:
          - stable
          - beta
          - nightly
`
	inputs, ok, err := ParseWorkflowDispatchInputs(raw)
	if err != nil {
		t.Fatalf("ParseWorkflowDispatchInputs returned error: %v", err)
	}
	if !ok {
		t.Fatal("workflow_dispatch should be detected")
	}
	want := []model.WorkflowInput{
		{Name: "version", Description: "Version to release", Required: true, Type: "string"},
		{Name: "prerelease", Type: "boolean", Default: "false"},
		{Name: "channel", Required: true, Type: "choice", Default: "beta", Options: []string{"stable", "beta", "nightly"}},
	}
	if len(inputs) != len(want) {
		t.Fatalf("inputs = %#v, want %#v", inputs, want)
	}
	for i := range want {
		if inputs[i].Name != want[i].Name || inputs[i].Description != want[i].Description || inputs[i].Required != want[i].Required || inputs[i].Type != want[i].Type || inputs[i].Default != want[i].Default {
			t.Fatalf("input %d = %#v, want %#v", i, inputs[i], want[i])
		}
		if len(inputs[i].Options) != len(want[i].Options) {
			t.Fatalf("input %d options = %#v, want %#v", i, inputs[i].Options, want[i].Options)
		}
	}
}

func TestParseWorkflowDispatchDetectsScalarAndSequenceForms(t *testing.T) {
	for _, raw := range []string{
		`on: workflow_dispatch`,
		`on: [push, workflow_dispatch]`,
	} {
		_, ok, err := ParseWorkflowDispatchInputs(raw)
		if err != nil {
			t.Fatalf("parse %q returned error: %v", raw, err)
		}
		if !ok {
			t.Fatalf("workflow_dispatch not detected in %q", raw)
		}
	}
}

func TestParseWorkflowDispatchInputsAbsent(t *testing.T) {
	inputs, ok, err := ParseWorkflowDispatchInputs(`on: [push, pull_request]`)
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	if ok || len(inputs) != 0 {
		t.Fatalf("inputs=%#v ok=%v, want absent", inputs, ok)
	}
}
