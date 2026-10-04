package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bharatchat/backend/internal/features/user/domain"
	"github.com/bharatchat/backend/internal/features/user/service"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/internal/platform/validator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type exportRepositoryStub struct {
	domain.UserRepository
	write func(context.Context, string, io.Writer) (bool, error)
}

func (s exportRepositoryStub) WriteAccountExport(ctx context.Context, id string, w io.Writer) (bool, error) {
	return s.write(ctx, id, w)
}

func exportTestRouter(write func(context.Context, string, io.Writer) (bool, error)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(string(middleware.ContextKeyUserID), uuid.New()) })
	handler := NewUserHandler(service.NewUserService(exportRepositoryStub{write: write}), validator.New())
	handler.RegisterRoutes(router.Group(""))
	return router
}

func TestExportFailureDoesNotReturnPartialArchive(t *testing.T) {
	router := exportTestRouter(func(ctx context.Context, id string, w io.Writer) (bool, error) {
		_, err := w.Write([]byte("sensitive incomplete archive"))
		require.NoError(t, err)
		return false, errors.New("snapshot failed")
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/me/export", nil))
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.NotContains(t, response.Body.String(), "sensitive incomplete")
	require.Contains(t, response.Header().Get("Cache-Control"), "no-store")
}

func TestExportSizeLimitReturnsErrorBeforeDownload(t *testing.T) {
	router := exportTestRouter(func(ctx context.Context, id string, w io.Writer) (bool, error) {
		chunk := make([]byte, 1<<20)
		for i := 0; i <= 32; i++ {
			if _, err := w.Write(chunk); err != nil {
				return false, err
			}
		}
		return true, nil
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/me/export", nil))
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.Contains(t, response.Body.String(), "EXPORT_TOO_LARGE")
}

func TestExportNotFoundDoesNotSetAttachmentHeaders(t *testing.T) {
	router := exportTestRouter(func(context.Context, string, io.Writer) (bool, error) { return false, nil })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/me/export", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Empty(t, response.Header().Get("Content-Disposition"))
}
