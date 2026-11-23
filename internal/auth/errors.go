package auth

import "errors"

var (
	ErrWeakPassword = errors.New(
		"password must contain at least 12 characters",
	)
	ErrUnauthorized = errors.New("unauthorized")
)
