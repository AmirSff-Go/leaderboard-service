package domain

import (
	"time"

	"github.com/google/uuid" // You'll need to add this dependency
)

type Game struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	TokenVersion int       `json:"token_version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
