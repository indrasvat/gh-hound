package github

import (
	"context"
	"errors"
)

// AuthenticatedLogin implements usecase.ViewerProvider: GET /user on
// the same authenticated client the run requests use.
func (c *Client) AuthenticatedLogin(ctx context.Context) (string, error) {
	var payload struct {
		Login string `json:"login"`
	}
	if err := c.getJSON(ctx, "/user", nil, &payload); err != nil {
		return "", err
	}
	if payload.Login == "" {
		return "", errors.New("github /user returned no login")
	}
	return payload.Login, nil
}
