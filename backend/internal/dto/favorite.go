package dto

import (
	"time"

	"github.com/lp/campus-market/internal/model"
)

// CreateFavoriteRequest is the payload for favoriting a product.
type CreateFavoriteRequest struct {
	ProductID uint `json:"product_id" binding:"required,gt=0"`
}

// ListFavoriteQuery filters the favorite list by product status.
type ListFavoriteQuery struct {
	Status string `form:"status"`
}

// FavoriteItem is one favorite entry enriched with product data and the
// price-drop flag computed against the price at favorite time.
type FavoriteItem struct {
	ID              uint           `json:"id"`
	UserID          uint           `json:"user_id"`
	ProductID       uint           `json:"product_id"`
	PriceAtFavorite float64        `json:"price_at_favorite"`
	CurrentPrice    float64        `json:"current_price"`
	PriceDropped    bool           `json:"price_dropped"`
	CreatedAt       time.Time      `json:"created_at"`
	Product         *model.Product `json:"product"`
}

// UpdateProductPriceRequest is the payload for a seller price change.
type UpdateProductPriceRequest struct {
	Price float64 `json:"price" binding:"required,gt=0"`
}
