package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lp/campus-market/internal/constants"
	"github.com/lp/campus-market/internal/dto"
	"github.com/lp/campus-market/internal/middleware"
	"github.com/lp/campus-market/internal/service"
	"github.com/lp/campus-market/internal/util"
)

// FavoriteHandler exposes product favorite endpoints.
type FavoriteHandler struct {
	svc    *service.FavoriteService
	logger *slog.Logger
}

// NewFavoriteHandler wires the favorite handler dependencies.
func NewFavoriteHandler(svc *service.FavoriteService, logger *slog.Logger) *FavoriteHandler {
	return &FavoriteHandler{svc: svc, logger: logger}
}

// Add handles POST /favorites.
func (h *FavoriteHandler) Add(c *gin.Context) {
	userID, err := middleware.CurrentUserID(c)
	if err != nil {
		util.Fail(c, http.StatusUnauthorized, constants.CodeUnauthorized, constants.MsgUnauthorized)
		return
	}
	var req dto.CreateFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeValidation, constants.MsgValidationFailed)
		return
	}
	f, err := h.svc.Add(c.Request.Context(), userID, req.ProductID)
	if err != nil {
		c.Error(err)
		return
	}
	util.OK(c, f)
}

// Remove handles DELETE /favorites/:productId.
func (h *FavoriteHandler) Remove(c *gin.Context) {
	userID, err := middleware.CurrentUserID(c)
	if err != nil {
		util.Fail(c, http.StatusUnauthorized, constants.CodeUnauthorized, constants.MsgUnauthorized)
		return
	}
	productID, err := strconv.ParseUint(c.Param("productId"), 10, 64)
	if err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "商品ID不合法")
		return
	}
	if err := h.svc.Remove(c.Request.Context(), userID, uint(productID)); err != nil {
		c.Error(err)
		return
	}
	util.OK(c, gin.H{"product_id": uint(productID)})
}

// ListMine handles GET /favorites/me.
func (h *FavoriteHandler) ListMine(c *gin.Context) {
	userID, err := middleware.CurrentUserID(c)
	if err != nil {
		util.Fail(c, http.StatusUnauthorized, constants.CodeUnauthorized, constants.MsgUnauthorized)
		return
	}
	var q dto.ListFavoriteQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeValidation, constants.MsgValidationFailed)
		return
	}
	items, err := h.svc.ListMine(c.Request.Context(), userID, q.Status)
	if err != nil {
		c.Error(err)
		return
	}
	util.OK(c, items)
}
