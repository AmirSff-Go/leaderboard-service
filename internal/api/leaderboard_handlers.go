package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

type LeaderboardHandler struct {
	leaderboardRepo repository.LeaderboardRepo
}

func NewLeaderboardHandler(leaderboardRepo repository.LeaderboardRepo) *LeaderboardHandler {
	return &LeaderboardHandler{
		leaderboardRepo: leaderboardRepo,
	}
}

type CreateLeaderboardRequest struct {
	UniqueName      string `json:"unique_name"`
	Description     string `json:"description"`
	Type            string `json:"type"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func (h *LeaderboardHandler) CreateLeaderboard(c echo.Context) error {
	var req CreateLeaderboardRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request body")
	} else if req.UniqueName == "" {
		return respondError(c, http.StatusBadRequest, "unique_name is required")
	} else if req.Type == "" {
		return respondError(c, http.StatusBadRequest, "type is required")
	} else if req.IntervalSeconds <= 0 {
		return respondError(c, http.StatusBadRequest, "interval_seconds must be greater than 0")
	} else if !domain.IsValidLeaderboardType(req.Type) {
		return respondError(c, http.StatusBadRequest, "invalid type")
	}
	game := GetGameFromContext(c)
	leaderboard := &domain.Leaderboard{
		GameID:          game.ID,
		UniqueName:      req.UniqueName,
		Description:     req.Description,
		Type:            domain.LeaderboardType(req.Type),
		IntervalSeconds: req.IntervalSeconds,
	}
	if err := h.leaderboardRepo.Create(c.Request().Context(), leaderboard); err != nil {
		if err == repository.ErrDuplicateLeaderboardName {
			return respondError(c, http.StatusConflict, "leaderboard name already exists")
		}
		return respondError(c, http.StatusInternalServerError, "failed to create leaderboard")
	}
	return respondOK(c, http.StatusCreated, leaderboard)
}
