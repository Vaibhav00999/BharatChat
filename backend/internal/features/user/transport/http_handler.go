package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/bharatchat/backend/internal/features/user/domain"
	"github.com/bharatchat/backend/internal/features/user/service"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"net/http"
	"time"
)

type UserHandler struct {
	service     *service.UserService
	validator   *validator.Validate
	lookupLimit gin.HandlerFunc
	reportLimit gin.HandlerFunc
	exportLimit gin.HandlerFunc
	exportSlots chan struct{}
}

func NewUserHandler(s *service.UserService, v *validator.Validate) *UserHandler {
	return &UserHandler{service: s, validator: v, exportSlots: make(chan struct{}, 2)}
}
func (h *UserHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/users/me", h.GetMe)
	r.PATCH("/users/me", h.UpdateMe)
	r.PATCH("/users/me/privacy", h.UpdatePrivacy)
	exportHandlers := []gin.HandlerFunc{h.ExportAccount}
	if h.exportLimit != nil {
		exportHandlers = append([]gin.HandlerFunc{h.exportLimit}, exportHandlers...)
	}
	r.GET("/users/me/export", exportHandlers...)
	r.DELETE("/users/me", h.DeleteAccount)
	r.GET("/users/username-available", h.CheckUsernameAvailable)
	lookupHandlers := []gin.HandlerFunc{h.ResolveUsername}
	if h.lookupLimit != nil {
		lookupHandlers = append([]gin.HandlerFunc{h.lookupLimit}, lookupHandlers...)
	}
	r.POST("/users/resolve", lookupHandlers...)
	r.GET("/users/me/blocked", h.ListBlocked)
	r.PUT("/users/me/blocked/:userId", h.SetBlocked)
	r.DELETE("/users/me/blocked/:userId", h.SetBlocked)
	reportHandlers := []gin.HandlerFunc{h.ReportUser}
	if h.reportLimit != nil {
		reportHandlers = append([]gin.HandlerFunc{h.reportLimit}, reportHandlers...)
	}
	r.POST("/users/me/reports", reportHandlers...)
}

const maxAccountExportBytes = 32 << 20

var errAccountExportTooLarge = errors.New("account export exceeds size limit")

type accountExportBuffer struct{ bytes.Buffer }

func (b *accountExportBuffer) Write(p []byte) (int, error) {
	if len(p) > maxAccountExportBytes-b.Len() {
		return 0, errAccountExportTooLarge
	}
	return b.Buffer.Write(p)
}

func (h *UserHandler) ExportAccount(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	select {
	case h.exportSlots <- struct{}{}:
		defer func() { <-h.exportSlots }()
	default:
		c.Header("Retry-After", "30")
		respondError(c, apperror.New("EXPORT_BUSY", "account export is busy, please retry later", http.StatusServiceUnavailable, nil))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	// Complete the bounded archive in memory before committing an HTTP 200. No
	// sensitive temporary files or successful-but-truncated downloads are created.
	var archive accountExportBuffer
	if err := h.service.ExportAccount(ctx, middleware.UserIDFromContext(c).String(), &archive); err != nil {
		if errors.Is(err, errAccountExportTooLarge) {
			respondError(c, apperror.New("EXPORT_TOO_LARGE", "account export exceeds the download limit", http.StatusRequestEntityTooLarge, nil))
		} else {
			respondError(c, err)
		}
		return
	}
	filename := fmt.Sprintf("bharatchat-account-export-%s.zip", time.Now().UTC().Format("20060102T150405Z"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/zip", archive.Bytes())
}

func (h *UserHandler) WithAccountExportLimit(limit gin.HandlerFunc) *UserHandler {
	h.exportLimit = limit
	return h
}

func (h *UserHandler) WithSafetyLimits(lookup, report gin.HandlerFunc) *UserHandler {
	h.lookupLimit, h.reportLimit = lookup, report
	return h
}

func (h *UserHandler) DeleteAccount(c *gin.Context) {
	var req DeleteAccountRequest
	if c.ShouldBindJSON(&req) != nil || h.validator.Struct(req) != nil {
		respondError(c, apperror.NewValidation("type DELETE to confirm account deletion"))
		return
	}
	if err := h.service.DeleteAccount(
		c.Request.Context(),
		middleware.UserIDFromContext(c).String(),
		middleware.SessionIDFromContext(c).String(),
		req.RefreshToken,
		req.Confirmation,
	); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func privacyLevel(value *string) *domain.PrivacyLevel {
	if value == nil {
		return nil
	}
	level := domain.PrivacyLevel(*value)
	return &level
}

func (h *UserHandler) UpdatePrivacy(c *gin.Context) {
	var req UpdatePrivacyRequest
	if c.ShouldBindJSON(&req) != nil || h.validator.Struct(req) != nil {
		respondError(c, apperror.NewValidation("invalid privacy settings"))
		return
	}
	user, err := h.service.UpdatePrivacy(c.Request.Context(), middleware.UserIDFromContext(c).String(), domain.PrivacyUpdate{
		LastSeen: privacyLevel(req.LastSeen), Avatar: privacyLevel(req.Avatar), About: privacyLevel(req.About),
		Phone: privacyLevel(req.Phone), AllowGroupAdds: privacyLevel(req.AllowGroupAdds),
		ReadReceipts: req.ReadReceipts, DiscoverableByPhone: req.DiscoverableByPhone,
		SecurityNotifications: req.SecurityNotifications, ShareTypingIndicators: req.ShareTypingIndicators,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToUserResponse(user))
}
func respondError(c *gin.Context, err error) {
	if a, ok := apperror.As(err); ok {
		c.JSON(a.HTTPStatus, gin.H{"code": a.Code, "message": a.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "an unexpected error occurred"})
}
func (h *UserHandler) GetMe(c *gin.Context) {
	u, err := h.service.GetProfile(c.Request.Context(), middleware.UserIDFromContext(c).String())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToUserResponse(u))
}
func (h *UserHandler) UpdateMe(c *gin.Context) {
	var req UpdateProfileRequest
	if c.ShouldBindJSON(&req) != nil || h.validator.Struct(req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	u, err := h.service.UpdateProfile(c.Request.Context(), middleware.UserIDFromContext(c).String(), domain.ProfileUpdate{DisplayName: req.DisplayName, Username: req.Username, About: req.About, AvatarURL: req.AvatarURL})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToUserResponse(u))
}
func (h *UserHandler) CheckUsernameAvailable(c *gin.Context) {
	name := c.Query("username")
	if name == "" {
		respondError(c, apperror.NewValidation("query parameter 'username' is required"))
		return
	}
	available, err := h.service.IsUsernameAvailable(c.Request.Context(), name)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, UsernameAvailabilityResponse{Username: name, Available: available})
}
