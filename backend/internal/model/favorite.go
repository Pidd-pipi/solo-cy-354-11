package model

import "time"

// Favorite is a buyer's bookmark of a product, recording the price at
// favorite time so price drops can be detected later.
type Favorite struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	UserID          uint      `gorm:"uniqueIndex:idx_favorites_user_product;index;not null" json:"user_id"`
	ProductID       uint      `gorm:"uniqueIndex:idx_favorites_user_product;index;not null" json:"product_id"`
	PriceAtFavorite float64   `gorm:"not null" json:"price_at_favorite"`
	CreatedAt       time.Time `json:"created_at"`
}
