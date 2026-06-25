package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/AmirSff-Go/leaderboard-service/internal/auth"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

type AdminHandler struct {
	adminAuth      *auth.AdminAuth
	tokenGenerator *auth.TokenGenerator
	gameRepo       repository.GameRepo
}

func NewAdminHandler(
	adminAuth *auth.AdminAuth,
	tokenGenerator *auth.TokenGenerator,
	gameRepo repository.GameRepo,
) *AdminHandler {
	return &AdminHandler{
		adminAuth:      adminAuth,
		tokenGenerator: tokenGenerator,
		gameRepo:       gameRepo,
	}
}

// RegisterGameRequest is the request body for creating a new game.
type RegisterGameRequest struct {
	AdminPassword string `json:"admin_password"`
	GameName      string `json:"game_name"`
	GameDesc      string `json:"game_desc"`
}

// RegisterGame creates a new game and returns a JWT token.
// POST /admin/games
func (h *AdminHandler) RegisterGame(c echo.Context) error {
	var req RegisterGameRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request body")
	}

	if err := h.adminAuth.ValidatePassword(req.AdminPassword); err != nil {
		return respondError(c, http.StatusUnauthorized, "invalid admin password")
	}

	if req.GameName == "" {
		return respondError(c, http.StatusBadRequest, "game_name is required")
	}

	game := &domain.Game{
		ID:           uuid.New(),
		Name:         req.GameName,
		Description:  req.GameDesc,
		TokenVersion: 1,
	}

	if err := h.gameRepo.Create(context.Background(), game); err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to create game")
	}

	token, err := h.tokenGenerator.GenerateToken(game.ID.String(), game.TokenVersion)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to generate token")
	}

	return respondOK(c, http.StatusCreated, GameResponse{
		ID:          game.ID.String(),
		Name:        game.Name,
		Description: game.Description,
		Token:       token,
		CreatedAt:   game.CreatedAt.String(),
		UpdatedAt:   game.UpdatedAt.String(),
	})
}

// RefreshTokenRequest is the request body for refreshing a game token.
type RefreshTokenRequest struct {
	AdminPassword string `json:"admin_password"`
	GameID        string `json:"game_id"`
}

// RefreshGameToken increments token_version and returns a new JWT token.
// POST /admin/games/:id/refresh-token
func (h *AdminHandler) RefreshGameToken(c echo.Context) error {
	var req RefreshTokenRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request body")
	}

	if err := h.adminAuth.ValidatePassword(req.AdminPassword); err != nil {
		return respondError(c, http.StatusUnauthorized, "invalid admin password")
	}

	gameID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return respondError(c, http.StatusBadRequest, "invalid game id")
	}

	game, err := h.gameRepo.GetByID(context.Background(), gameID)
	if errors.Is(err, repository.ErrGameNotFound) {
		return respondError(c, http.StatusNotFound, "game not found")
	}
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "database error")
	}

	game.TokenVersion++
	if err := h.gameRepo.Update(context.Background(), game); err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to update game")
	}

	token, err := h.tokenGenerator.GenerateToken(game.ID.String(), game.TokenVersion)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to generate token")
	}

	return respondOK(c, http.StatusOK, GameResponse{
		ID:          game.ID.String(),
		Name:        game.Name,
		Description: game.Description,
		Token:       token,
		CreatedAt:   game.CreatedAt.String(),
		UpdatedAt:   game.UpdatedAt.String(),
	})
}

// EditGameRequest is the request body for editing a game.
type EditGameRequest struct {
	AdminPassword string `json:"admin_password"`
	GameName      string `json:"game_name"`
	GameDesc      string `json:"game_desc"`
}

// EditGame updates game name and description.
// PATCH /admin/games/:id
func (h *AdminHandler) EditGame(c echo.Context) error {
	var req EditGameRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request body")
	}

	if err := h.adminAuth.ValidatePassword(req.AdminPassword); err != nil {
		return respondError(c, http.StatusUnauthorized, "invalid admin password")
	}

	gameID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return respondError(c, http.StatusBadRequest, "invalid game id")
	}

	game, err := h.gameRepo.GetByID(context.Background(), gameID)
	if errors.Is(err, repository.ErrGameNotFound) {
		return respondError(c, http.StatusNotFound, "game not found")
	}
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "database error")
	}

	if req.GameName != "" {
		game.Name = req.GameName
	}
	game.Description = req.GameDesc

	if err := h.gameRepo.Update(context.Background(), game); err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to update game")
	}

	return respondOK(c, http.StatusOK, GameResponse{
		ID:          game.ID.String(),
		Name:        game.Name,
		Description: game.Description,
		CreatedAt:   game.CreatedAt.String(),
		UpdatedAt:   game.UpdatedAt.String(),
	})
}
