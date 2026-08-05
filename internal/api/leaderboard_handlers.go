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
	} else if req.IntervalSeconds < 0 {
		return respondError(c, http.StatusBadRequest, "interval_seconds must be zero or greater")
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

// maxPageSize bounds how many rows a single rankings request can request. Beyond keeping
// responses reasonably sized, GetRankings issues one rank lookup per distinct score in the
// page, so an unbounded page_size turns one HTTP request into an unbounded number of
// sequential repository calls.
const maxPageSize = 100

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
// @Param       page           query int    false "Page number, 1-based (default: 1)"
// @Param       page_size      query int    false "Results per page, 1-100 (default: 20)"
// @Param       user_id        query string false "Include this user's rank and score in user_entry"
// @Param       duration_index query int    false "Time bucket (-1 = current period, 0+ = historical)"
// @Success     200 {object} GetRankingsResponseBody
// @Failure     400 {object} ErrorResponse "page less than 1, or page_size outside 1-100"
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

	if page < 1 {
		return respondError(c, http.StatusBadRequest, "page must be 1 or greater")
	}
	if pageSize < 1 {
		return respondError(c, http.StatusBadRequest, "page_size must be 1 or greater")
	}
	if pageSize > maxPageSize {
		return respondError(c, http.StatusBadRequest, fmt.Sprintf("page_size must be %d or less", maxPageSize))
	}

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

// @Summary     List leaderboards
// @Description Lists every leaderboard belonging to the authenticated game.
// @Tags        Leaderboards
// @Produce     json
// @Security    BearerAuth
// @Success     200 {array} domain.Leaderboard
// @Failure     401 {object} ErrorResponse "invalid or missing token"
// @Failure     500 {object} ErrorResponse
// @Router      /leaderboards [get]
func (h *LeaderboardHandler) ListLeaderboards(c echo.Context) error {
	game := GetGameFromContext(c)
	leaderboards, err := h.leaderboardService.ListLeaderboards(c.Request().Context(), game.ID)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, "failed to list leaderboards")
	}
	return respondOK(c, http.StatusOK, leaderboards)
}

type UpdateLeaderboardRequest struct {
	UniqueName  string `json:"unique_name"`
	Description string `json:"description"`
}

// @Summary     Rename or redescribe a leaderboard
// @Description Updates unique_name and/or description. Both are optional; omit either to keep its current value. Type and interval_seconds cannot be changed here — create a new leaderboard instead.
// @Tags        Leaderboards
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       name    path string                    true "Leaderboard unique name"
// @Param       request body UpdateLeaderboardRequest true "Fields to update"
// @Success     200 {object} domain.Leaderboard
// @Failure     401 {object} ErrorResponse "invalid or missing token"
// @Failure     404 {object} ErrorResponse "leaderboard not found"
// @Failure     409 {object} ErrorResponse "unique_name already exists for this game"
// @Failure     500 {object} ErrorResponse
// @Router      /leaderboards/{name} [patch]
func (h *LeaderboardHandler) UpdateLeaderboard(c echo.Context) error {
	var req UpdateLeaderboardRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request body")
	}
	game := GetGameFromContext(c)
	leaderboardName := c.Param("name")

	leaderboard, err := h.leaderboardService.UpdateLeaderboard(c.Request().Context(), game.ID, leaderboardName, req.UniqueName, req.Description)
	if err != nil {
		if errors.Is(err, domain.ErrLeaderboardNotFound) {
			return respondError(c, http.StatusNotFound, "leaderboard not found")
		}
		if errors.Is(err, domain.ErrDuplicateLeaderboardName) {
			return respondError(c, http.StatusConflict, "leaderboard name already exists for this game")
		}
		return respondError(c, http.StatusInternalServerError, "failed to update leaderboard")
	}
	return respondOK(c, http.StatusOK, leaderboard)
}

// @Summary     Delete a leaderboard
// @Description Permanently deletes a leaderboard and all of its scores, across every period.
// @Tags        Leaderboards
// @Security    BearerAuth
// @Param       name path string true "Leaderboard unique name"
// @Success     204 "leaderboard deleted"
// @Failure     401 {object} ErrorResponse "invalid or missing token"
// @Failure     404 {object} ErrorResponse "leaderboard not found"
// @Failure     500 {object} ErrorResponse
// @Router      /leaderboards/{name} [delete]
func (h *LeaderboardHandler) DeleteLeaderboard(c echo.Context) error {
	game := GetGameFromContext(c)
	leaderboardName := c.Param("name")

	err := h.leaderboardService.DeleteLeaderboard(c.Request().Context(), game.ID, leaderboardName)
	if err != nil {
		if errors.Is(err, domain.ErrLeaderboardNotFound) {
			return respondError(c, http.StatusNotFound, "leaderboard not found")
		}
		return respondError(c, http.StatusInternalServerError, "failed to delete leaderboard")
	}
	return c.NoContent(http.StatusNoContent)
}

type SetScoreRequest struct {
	Score int `json:"score"`
}

// @Summary     Set a user's score directly
// @Description Overwrites a user's score for one period, bypassing the leaderboard type's normal record/additive/onetime processing. For organizer corrections, not game clients reporting attempts.
// @Tags        Leaderboards
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       name           path string          true  "Leaderboard unique name"
// @Param       user_id        path string          true  "User ID"
// @Param       duration_index query int             false "Time bucket to edit (-1 = current period, default)"
// @Param       request        body SetScoreRequest true  "New score value"
// @Success     200 "score set"
// @Failure     400 {object} ErrorResponse "invalid request body"
// @Failure     401 {object} ErrorResponse "invalid or missing token"
// @Failure     404 {object} ErrorResponse "leaderboard not found"
// @Failure     500 {object} ErrorResponse
// @Router      /leaderboards/{name}/scores/{user_id} [patch]
func (h *LeaderboardHandler) SetScore(c echo.Context) error {
	var req SetScoreRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request body")
	}
	durationIndex, err := GetIntQueryParam(c, "duration_index", -1)
	if err != nil {
		return respondError(c, http.StatusBadRequest, "duration_index must be an integer")
	}

	game := GetGameFromContext(c)
	leaderboardName := c.Param("name")
	userID := c.Param("user_id")

	err = h.leaderboardService.SetScore(c.Request().Context(), game.ID, leaderboardName, userID, req.Score, durationIndex)
	if err != nil {
		if errors.Is(err, domain.ErrLeaderboardNotFound) {
			return respondError(c, http.StatusNotFound, "leaderboard not found")
		}
		return respondError(c, http.StatusInternalServerError, "failed to set score")
	}
	return respondOK(c, http.StatusOK, nil)
}

// @Summary     Delete a user's score
// @Description Removes a user's score for one period. Use to correct accidental submissions or remove a participant.
// @Tags        Leaderboards
// @Security    BearerAuth
// @Param       name           path string true  "Leaderboard unique name"
// @Param       user_id        path string true  "User ID"
// @Param       duration_index query int    false "Time bucket to delete from (-1 = current period, default)"
// @Success     204 "score deleted"
// @Failure     401 {object} ErrorResponse "invalid or missing token"
// @Failure     404 {object} ErrorResponse "leaderboard or score not found"
// @Failure     500 {object} ErrorResponse
// @Router      /leaderboards/{name}/scores/{user_id} [delete]
func (h *LeaderboardHandler) DeleteScore(c echo.Context) error {
	durationIndex, err := GetIntQueryParam(c, "duration_index", -1)
	if err != nil {
		return respondError(c, http.StatusBadRequest, "duration_index must be an integer")
	}

	game := GetGameFromContext(c)
	leaderboardName := c.Param("name")
	userID := c.Param("user_id")

	err = h.leaderboardService.DeleteScore(c.Request().Context(), game.ID, leaderboardName, userID, durationIndex)
	if err != nil {
		if errors.Is(err, domain.ErrLeaderboardNotFound) {
			return respondError(c, http.StatusNotFound, "leaderboard not found")
		}
		if errors.Is(err, domain.ErrScoreNotFound) {
			return respondError(c, http.StatusNotFound, "score not found")
		}
		return respondError(c, http.StatusInternalServerError, "failed to delete score")
	}
	return c.NoContent(http.StatusNoContent)
}
