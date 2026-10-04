package transport

import (
	"context"
	"github.com/bharatchat/backend/internal/features/chat/service"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"net/http"
)

type ChatHandler struct {
	service   *service.ChatService
	validator *validator.Validate
}

func NewChatHandler(s *service.ChatService, v *validator.Validate) *ChatHandler {
	return &ChatHandler{service: s, validator: v}
}
func (h *ChatHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/chats", h.ListChats)
	r.POST("/chats/direct", h.StartDirectChat)
	r.PATCH("/chats/:chatId/mute", h.SetMuted)
	r.PATCH("/chats/:chatId/pin", h.SetPinned)
	r.PATCH("/chats/:chatId/archive", h.SetArchived)
}
func respondError(c *gin.Context, err error) {
	if a, ok := apperror.As(err); ok {
		c.JSON(a.HTTPStatus, gin.H{"code": a.Code, "message": a.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "an unexpected error occurred"})
}
func (h *ChatHandler) ListChats(c *gin.Context) {
	v, err := h.service.ListChats(c.Request.Context(), middleware.UserIDFromContext(c).String())
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]ChatSummaryResponse, 0, len(v))
	for _, s := range v {
		out = append(out, ToChatSummaryResponse(s))
	}
	c.JSON(http.StatusOK, gin.H{"chats": out})
}
func (h *ChatHandler) StartDirectChat(c *gin.Context) {
	var req StartDirectChatRequest
	if c.ShouldBindJSON(&req) != nil || h.validator.Struct(req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	out, err := h.service.StartDirectChat(c.Request.Context(), middleware.UserIDFromContext(c).String(), req.PeerUserID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToChatResponse(out))
}
func (h *ChatHandler) flag(c *gin.Context, set func(context.Context, string, string, bool) error) {
	var req SetChatFlagRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	if err := set(c.Request.Context(), c.Param("chatId"), middleware.UserIDFromContext(c).String(), req.Value); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}
func (h *ChatHandler) SetMuted(c *gin.Context)    { h.flag(c, h.service.SetMuted) }
func (h *ChatHandler) SetPinned(c *gin.Context)   { h.flag(c, h.service.SetPinned) }
func (h *ChatHandler) SetArchived(c *gin.Context) { h.flag(c, h.service.SetArchived) }
