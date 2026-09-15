package repository

import (
	"context"

	"github.com/lp/campus-market/internal/model"
	"github.com/lp/campus-market/internal/util"
	"gorm.io/gorm"
)

// FavoriteRepository persists favorite rows.
type FavoriteRepository struct {
	db *gorm.DB
}

// NewFavoriteRepository builds a FavoriteRepository.
func NewFavoriteRepository(db *gorm.DB) *FavoriteRepository {
	return &FavoriteRepository{db: db}
}

// Create inserts a new favorite.
func (r *FavoriteRepository) Create(ctx context.Context, f *model.Favorite) error {
	return db(ctx, r.db).Create(f).Error
}

// FindByUserAndProduct returns the favorite of one user for one product.
func (r *FavoriteRepository) FindByUserAndProduct(ctx context.Context, userID, productID uint) (*model.Favorite, error) {
	var f model.Favorite
	err := db(ctx, r.db).Where("user_id = ? AND product_id = ?", userID, productID).First(&f).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &f, nil
}

// Delete removes the favorite of one user for one product.
func (r *FavoriteRepository) Delete(ctx context.Context, userID, productID uint) error {
	res := db(ctx, r.db).Where("user_id = ? AND product_id = ?", userID, productID).Delete(&model.Favorite{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return util.ErrNotFound
	}
	return nil
}

// ListByUser returns the user's favorites joined with their products,
// newest first. status optionally filters by the product's current status.
func (r *FavoriteRepository) ListByUser(ctx context.Context, userID uint, status string) ([]model.Favorite, []model.Product, error) {
	q := db(ctx, r.db).Table("favorites").
		Select("favorites.*").
		Joins("JOIN products ON products.id = favorites.product_id").
		Where("favorites.user_id = ?", userID)
	if status != "" {
		q = q.Where("products.status = ?", status)
	}
	var favorites []model.Favorite
	if err := q.Order("favorites.created_at DESC").Scan(&favorites).Error; err != nil {
		return nil, nil, err
	}
	if len(favorites) == 0 {
		return favorites, nil, nil
	}
	ids := make([]uint, 0, len(favorites))
	for _, f := range favorites {
		ids = append(ids, f.ProductID)
	}
	var products []model.Product
	if err := db(ctx, r.db).Where("id IN ?", ids).Find(&products).Error; err != nil {
		return nil, nil, err
	}
	return favorites, products, nil
}
