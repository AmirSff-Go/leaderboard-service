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

// GET /health/live → always 200
func (h *HealthHandler) Live(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}

// GET /health/ready → pings db + redis, returns 200 or 503
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
