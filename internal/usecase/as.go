package usecase

import "errors"

func as(err error, target any) bool {
	return errors.As(err, target)
}
