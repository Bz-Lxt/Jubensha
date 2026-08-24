package middleware_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/alkaid/jubensha-carpool/backend/internal/api/middleware"
)

type stalledPipelineHook struct{}

func (stalledPipelineHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (stalledPipelineHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }

func (stalledPipelineHook) ProcessPipelineHook(_ redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, _ []redis.Cmder) error {
		<-ctx.Done()
		return ctx.Err()
	}
}

func TestRateLimit_RedisTimeoutDoesNotCancelHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rdb := redis.NewClient(&redis.Options{Addr: "unused:6379"})
	rdb.AddHook(stalledPipelineHook{})
	t.Cleanup(func() { _ = rdb.Close() })

	router := gin.New()
	router.Use(middleware.RateLimit(rdb, "login", 1, time.Minute, middleware.ScopeIP))
	router.POST("/login", func(c *gin.Context) {
		if err := c.Request.Context().Err(); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = net.JoinHostPort("203.0.113.9", "41000")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("request returned %d after the limiter timed out, want %d; body=%s",
			res.Code, http.StatusNoContent, res.Body.String())
	}
}
