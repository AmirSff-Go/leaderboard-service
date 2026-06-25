package api

import (
	"errors"
	"fmt"
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

// @Summary     Create a leaderboard
// @Description Creates a new leaderboard scoped to the authenticated game. unique_name must be unique within the game.
// @Description Type options: record (personal best), additive (cumulative total), onetime (first submission only).
// @Description Set interval_seconds to 0 for all-time, 86400 for daily, 604800 for weekly.
// @Tags        Leaderboards
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       request body CreateLeaderboardRequest true "Leaderboard configuration"
// @Success     201 {object} domain.Leaderboard "leaderboard created"
// @Failure     400 {object} ErrorResponse "missing fields or invalid type"
// @Failure     401 {object} ErrorResponse "invalid or missing token"
// @Failure     409 {object} ErrorResponse "leaderboard name already exists for this game"
// @Failure     500 {object} ErrorResponse
// @Router      /leaderboards [post]
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
		if errors.Is(err, domain.ErrDuplicateLeaderboardName) {
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

// @Summary     Submit a score
// @Description Records a score for a user. Behavior depends on leaderboard type: record keeps the personal best, additive accumulates all submissions, onetime ignores submissions after the first.
// @Tags        Leaderboards
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       name    path string           true "Leaderboard unique name"
// @Param       request body SubmitScoreRequest true "Score submission"
// @Success     201 "score recorded"
// @Failure     400 {object} ErrorResponse "missing user_id"
// @Failure     401 {object} ErrorResponse "invalid or missing token"
// @Failure     404 {object} ErrorResponse "leaderboard not found"
// @Failure     500 {object} ErrorResponse
// @Router      /leaderboards/{name}/scores [post]
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
		if errors.Is(err, domain.ErrLeaderboardNotFound) {
			return respondError(c, http.StatusNotFound, "leaderboard not found")
		}
		return respondError(c, http.StatusInternalServerError, "failed to submit score")
	}

	return respondOK(c, http.StatusCreated, nil)
}

type GetRankingsResponseBody struct {
	Rankings  []*domain.ScoreObject `json:"rankings"`
	Total     int                   `json:"total"`
	Page      int                   `json:"page"`
	PageSize  int                   `json:"page_size"`
	UserEntry *domain.ScoreObject   `json:"user_entry,omitempty"`
}

// @Summary     Get rankings
// @Description Returns paginated rankings for a leaderboard. Optionally fetches the requesting user's rank via user_id.
// @Tags        Leaderboards
// @Produce     json
// @Security    BearerAuth
// @Param       name           path  string true  "Leaderboard unique name"
// @Param       page           query int    false "Page number (default: 1)"
// @Param       page_size      query int    false "Results per page (default: 20)"
// @Param       user_id        query string false "Include this user's rank and score in user_entry"
// @Param       duration_index query int    false "Time bucket (-1 = current period, 0+ = historical)"
// @Success     200 {object} GetRankingsResponseBody
// @Failure     401 {object} ErrorResponse "invalid or missing token"
// @Failure     404 {object} ErrorResponse "leaderboard not found"
// @Failure     500 {object} ErrorResponse
// @Router      /leaderboards/{name}/rankings [get]
func (h *LeaderboardHandler) GetRankings(c echo.Context) error {
	leaderboardName := c.Param("name")
	page, _ := GetIntQueryParam(c, "page", 1)
	pageSize, _ := GetIntQueryParam(c, "page_size", 20)
	userId := c.QueryParam("user_id")
	durationIndex, _ := GetIntQueryParam(c, "duration_index", -1)

	game := GetGameFromContext(c)

	rankings, total, userEntry, err := h.leaderboardService.GetRankings(c.Request().Context(), game.ID, leaderboardName, page, pageSize, userId, durationIndex)
	if err != nil {
		if errors.Is(err, domain.ErrLeaderboardNotFound) {
			return respondError(c, http.StatusNotFound, "leaderboard not found")
		}
		return respondError(c, http.StatusInternalServerError, "failed to get rankings")
	}

	return respondOK(c, http.StatusOK, GetRankingsResponseBody{
		Rankings:  rankings,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
		UserEntry: userEntry,
	})
}

func GetIntQueryParam(c echo.Context, name string, defaultValue int) (int, error) {
	paramStr := c.QueryParam(name)
	if paramStr == "" {
		return defaultValue, nil
	}
	var param int
	_, err := fmt.Sscanf(paramStr, "%d", &param)
	if err != nil {
		return 0, err
	}
	return param, nil
}
