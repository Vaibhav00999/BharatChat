package transport

import (
	"github.com/bharatchat/backend/internal/features/message/service"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"strconv"
)

type MessageHandler struct{ service *service.MessageService }

func NewMessageHandler(s *service.MessageService) *MessageHandler { return &MessageHandler{service: s} }
func (h *MessageHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/chats/:chatId/messages", h.ListHistory)
	r.POST("/chats/:chatId/read", h.MarkChatRead)
}
func respondError(c *gin.Context, err error) {
	if a, ok := apperror.As(err); ok {
		c.JSON(a.HTTPStatus, gin.H{"code": a.Code, "message": a.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "an unexpected error occurred"})
}
func (h *MessageHandler) ListHistory(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			respondError(c, apperror.NewValidation("limit must be an integer"))
			return
		}
		limit = n
	}
	var before *string
	if v := c.Query("before"); v != "" {
		if _, err := uuid.Parse(v); err != nil {
			respondError(c, apperror.NewValidation("before must be a UUID"))
			return
		}
		before = &v
	}
	out, err := h.service.GetHistory(c.Request.Context(), c.Param("chatId"), middleware.UserIDFromContext(c).String(), limit, before)
	if err != nil {
		respondError(c, err)
		return
	}
	res := make([]MessageResponse, 0, len(out))
	for _, m := range out {
		res = append(res, ToMessageResponse(m))
	}
	c.JSON(http.StatusOK, gin.H{"messages": res})
}

type MarkChatReadRequest struct {
	UpToMessageID string `json:"upToMessageId"`
}

func (h *MessageHandler) MarkChatRead(c *gin.Context) {
	var req MarkChatReadRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	if _, err := uuid.Parse(req.UpToMessageID); err != nil {
		respondError(c, apperror.NewValidation("upToMessageId must be a UUID"))
		return
	}
	if err := h.service.MarkChatReadUpTo(c.Request.Context(), c.Param("chatId"), middleware.UserIDFromContext(c).String(), req.UpToMessageID); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "marked read"})
}
