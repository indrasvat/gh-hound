package usecase

import (
	"context"

	"github.com/indrasvat/gh-hound/internal/model"
)

type RunsService struct {
	GitHub GitHub
}

func (s RunsService) LoadRuns(ctx context.Context, filter RunFilter) ([]model.Run, error) {
	return s.GitHub.ListRuns(ctx, filter)
}
