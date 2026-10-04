package transport

import (
	"net/http"

	"github.com/bharatchat/backend/internal/features/privacy/service"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type PrivacyHandler struct {
	service   *service.PrivacyService
	validator *validator.Validate
}

func NewPrivacyHandler(service *service.PrivacyService, validator *validator.Validate) *PrivacyHandler {
	return &PrivacyHandler{service: service, validator: validator}
}

func (h *PrivacyHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.PUT("/privacy/keys", h.UploadKeyBundle)
	group.GET("/privacy/users/:userId/key-bundles", h.ClaimKeyBundles)
	group.GET("/privacy/devices", h.ListLinkedDevices)
	group.DELETE("/privacy/devices/:deviceId", h.RevokeLinkedDevice)
}

func privacyError(c *gin.Context, err error) {
	if appErr, ok := apperror.As(err); ok {
		c.JSON(appErr.HTTPStatus, gin.H{"code": appErr.Code, "message": appErr.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "an unexpected error occurred"})
}

func (h *PrivacyHandler) UploadKeyBundle(c *gin.Context) {
	var request UploadKeyBundleRequest
	if err := c.ShouldBindJSON(&request); err != nil || h.validator.Struct(request) != nil {
		privacyError(c, apperror.NewValidation("invalid key bundle"))
		return
	}
	bundle, err := request.ToDomain()
	if err != nil {
		privacyError(c, apperror.NewValidation("key fields must be valid base64url"))
		return
	}
	stored, err := h.service.UploadKeyBundle(c.Request.Context(), middleware.SessionIDFromContext(c).String(), middleware.UserIDFromContext(c).String(), bundle)
	if err != nil {
		privacyError(c, err)
		return
	}
	c.JSON(http.StatusOK, toKeyBundleResponse(*stored))
}

func (h *PrivacyHandler) ClaimKeyBundles(c *gin.Context) {
	targetUserID := c.Param("userId")
	if _, err := uuid.Parse(targetUserID); err != nil {
		privacyError(c, apperror.NewValidation("userId must be a UUID"))
		return
	}
	bundles, err := h.service.ClaimBundles(
		c.Request.Context(),
		middleware.SessionIDFromContext(c).String(),
		middleware.UserIDFromContext(c).String(),
		targetUserID,
	)
	if err != nil {
		privacyError(c, err)
		return
	}
	responses := make([]KeyBundleResponse, 0, len(bundles))
	for _, bundle := range bundles {
		responses = append(responses, toKeyBundleResponse(bundle))
	}
	c.JSON(http.StatusOK, gin.H{"keyBundles": responses})
}

func (h *PrivacyHandler) ListLinkedDevices(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	currentDeviceID, err := h.service.ResolveDeviceID(c.Request.Context(), middleware.SessionIDFromContext(c).String(), userID)
	if err != nil {
		privacyError(c, err)
		return
	}
	devices, err := h.service.ListLinkedDevices(c.Request.Context(), userID)
	if err != nil {
		privacyError(c, err)
		return
	}
	responses := make([]LinkedDeviceResponse, 0, len(devices))
	for _, device := range devices {
		responses = append(responses, LinkedDeviceResponse{
			DeviceID: device.DeviceID, Platform: device.Platform, DeviceName: device.DeviceName,
			LastActiveAt: device.LastActiveAt, HasKeyBundle: device.HasKeyBundle, ActiveSessions: device.ActiveSessions,
			IsCurrent: device.DeviceID == currentDeviceID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"devices": responses})
}

func (h *PrivacyHandler) RevokeLinkedDevice(c *gin.Context) {
	deviceID := c.Param("deviceId")
	if _, err := uuid.Parse(deviceID); err != nil {
		privacyError(c, apperror.NewValidation("deviceId must be a UUID"))
		return
	}
	err := h.service.RevokeLinkedDevice(c.Request.Context(), middleware.SessionIDFromContext(c).String(), middleware.UserIDFromContext(c).String(), deviceID)
	if err != nil {
		privacyError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
