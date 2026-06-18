package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
)

type LeaderboardHandler struct {
	leaderboardService *domain.LeaderboardService
}

func NewLeaderboardHandler(leaderboardService *domain.LeaderboardService) *LeaderboardHandler {
	return &LeaderboardHandler{
		leaderboardService: leaderboardService,
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
	lbType := domain.LeaderboardType(req.Type)
	leaderboard, err := h.leaderboardService.CreateLeaderboard(c.Request().Context(), game.ID, req.UniqueName, req.Description, lbType, req.IntervalSeconds)
	if err != nil {
		if err == domain.ErrDuplicateLeaderboardName {
			return respondError(c, http.StatusConflict, "leaderboard name already exists for this game")
		}
		return respondError(c, http.StatusInternalServerError, "failed to create leaderboard")
	}

	return respondOK(c, http.StatusCreated, leaderboard)
}

type SubmitScoreRequest struct {
	UserID string `json:"user_id"`
	Score  int    `json:"score"`
}

func (h *LeaderboardHandler) SubmitScore(c echo.Context) error {
	var req SubmitScoreRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request body")
	}
	if req.UserID == "" {
		return respondError(c, http.StatusBadRequest, "user_id is required")
	}

	game := GetGameFromContext(c)

	leaderboardName := c.Param("name")

	err := h.leaderboardService.SubmitScore(c.Request().Context(), game.ID, leaderboardName, req.UserID, req.Score)
	if err != nil {
		if err == domain.ErrLeaderboardNotFound {
			return respondError(c, http.StatusNotFound, "leaderboard not found")
		}
		return respondError(c, http.StatusInternalServerError, "failed to submit score")
	}

	return respondOK(c, http.StatusCreated, nil)
}
