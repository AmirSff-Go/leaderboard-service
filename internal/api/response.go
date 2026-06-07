package api

import (
	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

type GameResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Token        string `json:"token,omitempty"` // Only in register/refresh responses
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func respondError(c echo.Context, code int, message string) error {
	return c.JSON(code, ErrorResponse{Error: message})
}

func respondOK(c echo.Context, code int, data interface{}) error {
	return c.JSON(code, data)
}
