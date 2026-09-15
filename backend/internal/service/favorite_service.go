package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lp/campus-market/internal/constants"
	"github.com/lp/campus-market/internal/dto"
	"github.com/lp/campus-market/internal/model"
	"github.com/lp/campus-market/internal/util"
)

// FavoriteRepository is the data access contract for favorite rows.
type FavoriteRepository interface {
	Create(ctx context.Context, f *model.Favorite) error
	FindByUserAndProduct(ctx context.Context, userID, productID uint) (*model.Favorite, error)
	Delete(ctx context.Context, userID, productID uint) error
	ListByUser(ctx context.Context, userID uint, status string) ([]model.Favorite, []model.Product, error)
}

// FavoriteService manages product favorites and price-drop detection.
type FavoriteService struct {
	favorites FavoriteRepository
	products  ProductRepository
	logger    *slog.Logger
}

// NewFavoriteService wires the favorite service dependencies.
func NewFavoriteService(favorites FavoriteRepository, products ProductRepository, logger *slog.Logger) *FavoriteService {
	return &FavoriteService{favorites: favorites, products: products, logger: logger}
}

// Add favorites an on-sale product for a buyer. Re-favoriting the same
// product keeps the existing record instead of creating a duplicate.
func (s *FavoriteService) Add(ctx context.Context, userID, productID uint) (*model.Favorite, error) {
	p, err := s.products.FindByID(ctx, productID)
	if err != nil {
		return nil, util.WrapAppError(fmt.Errorf("favorite[user=%d] add find product[id=%d]: %w", userID, productID, err), 404, constants.CodeNotFound, constants.MsgNotFound)
	}
	if p.SellerID == userID {
		return nil, util.NewAppError(403, constants.CodeForbidden, constants.MsgFavoriteOwnProduct, nil)
	}
	if p.Status != constants.ProductStatusOnSale {
		return nil, util.NewAppError(409, constants.CodeConflict, constants.MsgProductNotOnSale, nil)
	}
	if existing, err := s.favorites.FindByUserAndProduct(ctx, userID, productID); err == nil {
		s.logger.Info(fmt.Sprintf(constants.LogFavoriteDuplicate, userID, productID))
		return existing, nil
	}
	f := &model.Favorite{UserID: userID, ProductID: productID, PriceAtFavorite: p.Price}
	if err := s.favorites.Create(ctx, f); err != nil {
		// A concurrent request may have inserted the same (user, product)
		// pair first; treat the existing row as the result instead of failing.
		if existing, ferr := s.favorites.FindByUserAndProduct(ctx, userID, productID); ferr == nil {
			s.logger.Info(fmt.Sprintf(constants.LogFavoriteDuplicate, userID, productID))
			return existing, nil
		}
		return nil, util.WrapAppError(fmt.Errorf("favorite[user=%d] add product[id=%d]: %w", userID, productID, err), 500, constants.CodeInternalError, constants.MsgInternalError)
	}
	if f.ID == 0 {
		// The unique index ignored our insert because a concurrent request
		// won the race; return the canonical row it created.
		existing, err := s.favorites.FindByUserAndProduct(ctx, userID, productID)
		if err != nil {
			return nil, util.WrapAppError(fmt.Errorf("favorite[user=%d] add refetch product[id=%d]: %w", userID, productID, err), 500, constants.CodeInternalError, constants.MsgInternalError)
		}
		s.logger.Info(fmt.Sprintf(constants.LogFavoriteDuplicate, userID, productID))
		return existing, nil
	}
	s.logger.Info(fmt.Sprintf(constants.LogFavoriteAddSuccess, f.ID, userID, productID))
	return f, nil
}

// Remove deletes the user's favorite of a product.
func (s *FavoriteService) Remove(ctx context.Context, userID, productID uint) error {
	if err := s.favorites.Delete(ctx, userID, productID); err != nil {
		return util.WrapAppError(fmt.Errorf("favorite[user=%d] remove product[id=%d]: %w", userID, productID, err), 404, constants.CodeNotFound, constants.MsgFavoriteNotFound)
	}
	s.logger.Info(fmt.Sprintf(constants.LogFavoriteRemoveSuccess, userID, productID))
	return nil
}

// ListMine returns the user's favorites joined with products, marking entries
// whose current price is below the price at favorite time.
func (s *FavoriteService) ListMine(ctx context.Context, userID uint, status string) ([]dto.FavoriteItem, error) {
	if status != "" && !constants.IsProductStatus(status) {
		return nil, util.NewAppError(400, constants.CodeValidation, "商品状态不合法", nil)
	}
	favorites, products, err := s.favorites.ListByUser(ctx, userID, status)
	if err != nil {
		return nil, util.WrapAppError(fmt.Errorf("favorite[user=%d] list: %w", userID, err), 500, constants.CodeInternalError, constants.MsgInternalError)
	}
	byID := make(map[uint]*model.Product, len(products))
	for i := range products {
		byID[products[i].ID] = &products[i]
	}
	items := make([]dto.FavoriteItem, 0, len(favorites))
	for _, f := range favorites {
		p := byID[f.ProductID]
		item := dto.FavoriteItem{
			ID:              f.ID,
			UserID:          f.UserID,
			ProductID:       f.ProductID,
			PriceAtFavorite: f.PriceAtFavorite,
			CreatedAt:       f.CreatedAt,
			Product:         p,
		}
		if p != nil {
			item.CurrentPrice = p.Price
			item.PriceDropped = p.Price < f.PriceAtFavorite
		}
		items = append(items, item)
	}
	return items, nil
}
