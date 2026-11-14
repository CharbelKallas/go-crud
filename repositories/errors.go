// Package repositories holds the MySQL data access code.
package repositories

import "errors"

var (
	// ErrNotFound is returned when the requested row does not exist.
	ErrNotFound = errors.New("not found")
	// ErrDuplicateEmail is returned when a user with the email already exists.
	ErrDuplicateEmail = errors.New("email already registered")
)
