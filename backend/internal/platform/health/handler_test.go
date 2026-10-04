package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bharatchat/backend/internal/platform/health"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestReadinessRequiresEveryDependency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/readyz", health.Readiness(zerolog.Nop(), time.Second, map[string]health.Check{
		"postgres": func(context.Context) error { return nil },
		"redis":    func(context.Context) error { return errors.New("unavailable") },
	}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"status":"unavailable"}`, recorder.Body.String())
}

func TestReadinessSucceedsWhenDependenciesRespond(t *testing.T) {
	router := gin.New()
	router.GET("/readyz", health.Readiness(zerolog.Nop(), time.Second, map[string]health.Check{
		"postgres": func(context.Context) error { return nil },
		"redis":    func(context.Context) error { return nil },
	}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"status":"ready"}`, recorder.Body.String())
}
