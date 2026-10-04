// internal/features/group/transport/http_handler.go
package transport

import (
	"net/http"

	"github.com/bharatchat/backend/internal/features/group/service"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type GroupHandler struct {
	service   *service.GroupService
	validator *validator.Validate
}

func NewGroupHandler(svc *service.GroupService, v *validator.Validate) *GroupHandler {
	return &GroupHandler{service: svc, validator: v}
}

func (h *GroupHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/groups", h.CreateGroup)
	rg.GET("/groups/:chatId", h.GetGroupInfo)
	rg.PATCH("/groups/:chatId", h.UpdateGroupInfo)
	rg.GET("/groups/:chatId/members", h.ListMembers)
	rg.POST("/groups/:chatId/members", h.AddMember)
	rg.DELETE("/groups/:chatId/members/:userId", h.RemoveMember)
	rg.PATCH("/groups/:chatId/members/:userId/admin", h.SetAdmin)
	rg.POST("/groups/:chatId/transfer-ownership", h.TransferOwnership)
	rg.POST("/groups/:chatId/leave", h.LeaveGroup)
	rg.PATCH("/groups/:chatId/only-admins-can-post", h.SetOnlyAdminsCanPost)
	rg.PATCH("/groups/:chatId/only-admins-can-edit-info", h.SetOnlyAdminsCanEditInfo)
	rg.POST("/groups/:chatId/invite-code/regenerate", h.RegenerateInviteCode)
	rg.PATCH("/groups/:chatId/invite-code/enabled", h.SetInviteCodeEnabled)
	rg.POST("/groups/join", h.JoinViaInviteCode)
}

func respondError(c *gin.Context, err error) {
	if appErr, ok := apperror.As(err); ok {
		c.JSON(appErr.HTTPStatus, gin.H{"code": appErr.Code, "message": appErr.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "an unexpected error occurred"})
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()

	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		respondError(c, apperror.NewValidation(err.Error()))
		return
	}

	group, err := h.service.CreateGroup(c.Request.Context(), service.CreateGroupInput{
		CreatorUserID: userID,
		Name:          req.Name,
		Description:   req.Description,
		MemberUserIDs: req.MemberUserIDs,
	})
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ToGroupInfoResponse(group))
}

func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	group, err := h.service.GetGroupInfo(c.Request.Context(), chatID, userID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToGroupInfoResponse(group))
}

func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	var req UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		respondError(c, apperror.NewValidation(err.Error()))
		return
	}

	updated, err := h.service.UpdateGroupInfo(c.Request.Context(), service.UpdateGroupInfoInput{
		ChatID:           chatID,
		RequestingUserID: userID,
		Name:             req.Name,
		Description:      req.Description,
		IconURL:          req.IconURL,
	})
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToGroupInfoResponse(updated))
}

func (h *GroupHandler) ListMembers(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	members, err := h.service.ListMembers(c.Request.Context(), chatID, userID)
	if err != nil {
		respondError(c, err)
		return
	}

	responses := make([]MemberResponse, 0, len(members))
	for _, m := range members {
		responses = append(responses, ToMemberResponse(m))
	}

	c.JSON(http.StatusOK, gin.H{"members": responses})
}

func (h *GroupHandler) AddMember(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		respondError(c, apperror.NewValidation(err.Error()))
		return
	}

	if err := h.service.AddMember(c.Request.Context(), chatID, userID, req.UserID); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member added"})
}

func (h *GroupHandler) RemoveMember(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")
	targetUserID := c.Param("userId")

	if err := h.service.RemoveMember(c.Request.Context(), chatID, userID, targetUserID); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member removed"})
}

func (h *GroupHandler) SetAdmin(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")
	targetUserID := c.Param("userId")

	var req SetAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}

	if err := h.service.SetAdmin(c.Request.Context(), chatID, userID, targetUserID, req.IsAdmin); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "role updated"})
}

func (h *GroupHandler) TransferOwnership(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	var req TransferOwnershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		respondError(c, apperror.NewValidation(err.Error()))
		return
	}

	if err := h.service.TransferOwnership(c.Request.Context(), chatID, userID, req.NewOwnerUserID); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ownership transferred"})
}

func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	if err := h.service.LeaveGroup(c.Request.Context(), chatID, userID); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "left group"})
}

func (h *GroupHandler) SetOnlyAdminsCanPost(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	var req SetGroupFlagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}

	if err := h.service.SetOnlyAdminsCanPost(c.Request.Context(), chatID, userID, req.Value); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (h *GroupHandler) SetOnlyAdminsCanEditInfo(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	var req SetGroupFlagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}

	if err := h.service.SetOnlyAdminsCanEditInfo(c.Request.Context(), chatID, userID, req.Value); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (h *GroupHandler) RegenerateInviteCode(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	code, err := h.service.RegenerateInviteCode(c.Request.Context(), chatID, userID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, InviteCodeResponse{InviteCode: code})
}

func (h *GroupHandler) SetInviteCodeEnabled(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	chatID := c.Param("chatId")

	var req SetGroupFlagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}

	if err := h.service.SetInviteCodeEnabled(c.Request.Context(), chatID, userID, req.Value); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (h *GroupHandler) JoinViaInviteCode(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()

	var req JoinViaInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperror.NewValidation("invalid request body"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		respondError(c, apperror.NewValidation(err.Error()))
		return
	}

	group, err := h.service.JoinViaInviteCode(c.Request.Context(), req.InviteCode, userID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToGroupInfoResponse(group))
}
