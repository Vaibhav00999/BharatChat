package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bharatchat/backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRequestIDIsServerGeneratedAndAvailableToHandlers(t *testing.T) {
	router := gin.New()
	router.Use(middleware.RequestID())
	var contextID string
	router.GET("/", func(c *gin.Context) {
		contextID = middleware.RequestIDFromContext(c)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-ID", "attacker-controlled")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	responseID := recorder.Header().Get("X-Request-ID")
	require.NotEqual(t, "attacker-controlled", responseID)
	require.Equal(t, contextID, responseID)
	_, err := uuid.Parse(responseID)
	require.NoError(t, err)
}
