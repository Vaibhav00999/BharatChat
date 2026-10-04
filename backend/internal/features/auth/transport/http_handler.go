package transport

import (
	"context"
	"errors"
	"github.com/bharatchat/backend/internal/features/auth/domain"
	"github.com/bharatchat/backend/internal/features/auth/service"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"net/http"
	"time"
)

type AuthHandler struct {
	service              *service.AuthService
	tokenParser          middleware.TokenParser
	validator            *validator.Validate
	accessTTL            time.Duration
	wsTickets            WebSocketTicketIssuer
	storeNetworkMetadata bool
}

type WebSocketTicketIssuer interface {
	Issue(ctx context.Context, userID, sessionID string) (string, time.Duration, error)
}

func NewAuthHandler(s *service.AuthService, p middleware.TokenParser, v *validator.Validate, ttl time.Duration, wsTickets ...WebSocketTicketIssuer) *AuthHandler {
	handler := &AuthHandler{service: s, tokenParser: p, validator: v, accessTTL: ttl}
	if len(wsTickets) > 0 {
		handler.wsTickets = wsTickets[0]
	}
	return handler
}

func (h *AuthHandler) StoreNetworkMetadata(enabled bool) *AuthHandler {
	h.storeNetworkMetadata = enabled
	return h
}
func (h *AuthHandler) RegisterPublicRoutes(r *gin.RouterGroup) {
	r.POST("/auth/otp/request", h.RequestOTP)
	r.POST("/auth/otp/verify", h.VerifyOTP)
	r.POST("/auth/refresh", h.RefreshToken)
}
func (h *AuthHandler) RegisterProtectedRoutes(r *gin.RouterGroup) {
	r.POST("/auth/logout", h.Logout)
	if h.wsTickets != nil {
		r.POST("/auth/ws-ticket", h.IssueWebSocketTicket)
	}
}
func respondError(c *gin.Context, err error) {
	if a, ok := apperror.As(err); ok {
		c.JSON(a.HTTPStatus, gin.H{"code": a.Code, "message": a.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "an unexpected error occurred"})
}
func (h *AuthHandler) RequestOTP(c *gin.Context) {
	var req RequestOTPRequest
	if c.ShouldBindJSON(&req) != nil || h.validator.Struct(req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	if err := h.service.RequestOTP(c.Request.Context(), req.PhoneNumber, req.CountryCode); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "OTP sent successfully"})
}
func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req VerifyOTPRequest
	if c.ShouldBindJSON(&req) != nil || h.validator.Struct(req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	ipAddress, userAgent := "", ""
	if h.storeNetworkMetadata {
		ipAddress, userAgent = c.ClientIP(), c.Request.UserAgent()
	}
	result, err := h.service.VerifyOTP(c.Request.Context(), service.VerifyOTPInput{PhoneNumber: req.PhoneNumber, CountryCode: req.CountryCode, Code: req.Code, Device: domain.DeviceInfo{Platform: domain.DevicePlatform(req.Device.Platform), DeviceName: req.Device.DeviceName, PushToken: req.Device.PushToken, AppVersion: req.Device.AppVersion, InstallationID: req.Device.InstallationID}, IPAddress: ipAddress, UserAgent: userAgent})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAuthResponse(result))
}
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if c.ShouldBindJSON(&req) != nil || h.validator.Struct(req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	result, err := h.service.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAuthResponse(result))
}
func (h *AuthHandler) Logout(c *gin.Context) {
	sid := middleware.SessionIDFromContext(c).String()
	header := c.GetHeader("Authorization")
	raw := ""
	if len(header) > 7 {
		raw = header[7:]
	}
	jti := ""
	if claims, err := h.tokenParser.ParseAccessToken(raw); err == nil {
		jti = claims.JTI
	}
	if err := h.service.Logout(c.Request.Context(), sid, jti, h.accessTTL); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

func (h *AuthHandler) IssueWebSocketTicket(c *gin.Context) {
	if h.wsTickets == nil {
		respondError(c, apperror.NewInternal(errors.New("WebSocket tickets are not configured")))
		return
	}
	ticket, ttl, err := h.wsTickets.Issue(c.Request.Context(), middleware.UserIDFromContext(c).String(), middleware.SessionIDFromContext(c).String())
	if err != nil {
		respondError(c, apperror.NewInternal(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ticket": ticket, "expiresInSeconds": int(ttl.Seconds())})
}
func toAuthResponse(r *domain.AuthResult) AuthResponse {
	return AuthResponse{UserID: r.UserID, IsNewUser: r.IsNewUser, AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, AccessTokenExpiresAt: r.AccessTokenExpiresAt.UTC().Format(time.RFC3339)}
}
