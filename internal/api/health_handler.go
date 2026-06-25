package api

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	db    *sql.DB
	redis *redis.Client // can be nil if Redis is degraded
}

func NewHealthHandler(db *sql.DB, redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{
		db:    db,
		redis: redisClient,
	}
}

// @Summary     Liveness probe
// @Description Always returns 200. Used by Kubernetes to confirm the process is running.
// @Tags        Health
// @Success     200 "alive"
// @Router      /health/live [get]
func (h *HealthHandler) Live(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}

// @Summary     Readiness probe
// @Description Pings PostgreSQL and Redis. Returns 503 if either dependency is unreachable.
// @Tags        Health
// @Produce     json
// @Success     200 {object} map[string]string "all dependencies healthy"
// @Failure     503 {object} map[string]string "one or more dependencies unreachable"
// @Router      /health/ready [get]
func (h *HealthHandler) Ready(c echo.Context) error {
	status := map[string]string{
		"db":    "ok",
		"redis": "ok",
	}
	code := 200

	if err := h.db.PingContext(c.Request().Context()); err != nil {
		status["db"] = "unreachable"
		code = 503
	}

	if h.redis != nil {
		if err := h.redis.Ping(c.Request().Context()).Err(); err != nil {
			status["redis"] = "unreachable"
			code = 503
		}
	} else {
		status["redis"] = "disabled"
	}

	return c.JSON(code, status)
}
