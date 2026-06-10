package usecase_test

import (
	"context"
	"testing"

	"github.com/indrasvat/gh-hound/internal/adapter/fake"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestFailureServiceReturnsParsedLogAndAnnotations(t *testing.T) {
	service := usecase.FailureService{GitHub: fake.New(fake.ScenarioFailing)}
	job := model.Job{ID: 399444496, Name: "build", CheckRunURL: "https://api.github.com/repos/indrasvat/gh-hound/check-runs/399444496"}

	report, err := service.LoadFailure(context.Background(), "indrasvat/gh-hound", job)
	if err != nil {
		t.Fatalf("LoadFailure returned error: %v", err)
	}
	if !report.Log.Failure.Found {
		t.Fatalf("failure not found: %#v", report.Log.Failure)
	}
	if len(report.Annotations) != 1 || report.Annotations[0].Path != "internal/parser/lexer.go" {
		t.Fatalf("annotations = %#v", report.Annotations)
	}
}
