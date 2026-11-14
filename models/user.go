package models

import (
	"golang.org/x/crypto/bcrypt"
)

// User is a stored account. Password holds the bcrypt hash, never the
// plain-text password; User is never bound from or rendered to JSON.
type User struct {
	ID       int64
	Name     string
	Email    string
	Password string
	// SessionVersion is incremented when all of the user's sessions are
	// revoked; tokens carrying an older version are rejected.
	SessionVersion int64
}

func (user *User) HashPassword(password string, cost int) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return err
	}
	user.Password = string(hash)
	return nil
}

func (user *User) CheckPassword(providedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(providedPassword))
}
