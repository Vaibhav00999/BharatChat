package transport

import (
	"net/http"

	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/gin-gonic/gin"
)

func (h *UserHandler) ResolveUsername(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	p, err := h.service.ResolveUsername(c.Request.Context(), middleware.UserIDFromContext(c).String(), req.Username)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *UserHandler) ListBlocked(c *gin.Context) {
	people, next, err := h.service.ListBlocked(c.Request.Context(), middleware.UserIDFromContext(c).String(), c.Query("after"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": people, "nextCursor": next})
}

func (h *UserHandler) SetBlocked(c *gin.Context) {
	err := h.service.SetBlocked(c.Request.Context(), middleware.UserIDFromContext(c).String(), c.Param("userId"), c.Request.Method == http.MethodPut)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UserHandler) ReportUser(c *gin.Context) {
	var req struct {
		UserID  string `json:"userId"`
		Reason  string `json:"reason"`
		Details string `json:"details"`
	}
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	id, err := h.service.ReportUser(c.Request.Context(), middleware.UserIDFromContext(c).String(), req.UserID, req.Reason, req.Details)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "status": "open"})
}
