package auth

import (
	"errors"
)

var ErrInvalidAdminPassword = errors.New("invalid admin password")

type AdminAuth struct {
	password string
}

func NewAdminAuth(password string) *AdminAuth {
	return &AdminAuth{password: password}
}

func (a *AdminAuth) ValidatePassword(provided string) error {
	if provided == "" {
		return ErrInvalidAdminPassword
	}
	if provided != a.password {
		return ErrInvalidAdminPassword
	}
	return nil
}
