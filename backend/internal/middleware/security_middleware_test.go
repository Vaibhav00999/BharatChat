package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bharatchat/backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLimitRequestBodyRejectsKnownOversizedPayload(t *testing.T) {
	router := gin.New()
	router.Use(middleware.LimitRequestBody())
	router.POST("/upload", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.JSONEq(t, `{"code":"PAYLOAD_TOO_LARGE","message":"request body is too large"}`, recorder.Body.String())
}

func TestSecurityHeadersEnableHSTSOnlyInProduction(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		production bool
		hasHSTS    bool
	}{{"development", false, false}, {"production", true, true}} {
		t.Run(testCase.name, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware.SecurityHeaders(testCase.production))
			router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, testCase.hasHSTS, recorder.Header().Get("Strict-Transport-Security") != "")
			require.Equal(t, "no-store, max-age=0", recorder.Header().Get("Cache-Control"))
		})
	}
}
