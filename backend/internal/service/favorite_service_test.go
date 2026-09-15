package service

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/lp/campus-market/internal/constants"
	"github.com/lp/campus-market/internal/model"
	"github.com/lp/campus-market/internal/util"
)

// fakeFavoriteRepo mimics the real repository's INSERT IGNORE semantics: a
// conflicting insert is a no-op that leaves the model ID at zero.
type fakeFavoriteRepo struct {
	mu        sync.Mutex
	favorites map[uint]*model.Favorite
	nextID    uint
}

func newFakeFavoriteRepo() *fakeFavoriteRepo {
	return &fakeFavoriteRepo{favorites: map[uint]*model.Favorite{}, nextID: 1}
}

func (f *fakeFavoriteRepo) Create(_ context.Context, fav *model.Favorite) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.favorites {
		if existing.UserID == fav.UserID && existing.ProductID == fav.ProductID {
			return nil // unique index ignored the insert; ID stays zero
		}
	}
	fav.ID = f.nextID
	f.nextID++
	f.favorites[fav.ID] = fav
	return nil
}

func (f *fakeFavoriteRepo) FindByUserAndProduct(_ context.Context, userID, productID uint) (*model.Favorite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, fav := range f.favorites {
		if fav.UserID == userID && fav.ProductID == productID {
			cp := *fav
			return &cp, nil
		}
	}
	return nil, util.ErrNotFound
}

func (f *fakeFavoriteRepo) Delete(_ context.Context, userID, productID uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, fav := range f.favorites {
		if fav.UserID == userID && fav.ProductID == productID {
			delete(f.favorites, id)
			return nil
		}
	}
	return util.ErrNotFound
}

func (f *fakeFavoriteRepo) ListByUser(_ context.Context, userID uint, status string) ([]model.Favorite, []model.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.Favorite
	for _, fav := range f.favorites {
		if fav.UserID == userID {
			out = append(out, *fav)
		}
	}
	return out, nil, nil
}

func (f *fakeFavoriteRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.favorites)
}

// favoriteProductRepo adapts fakeProductRepo so ListByUser results can be
// joined with products by the service under test.
func setupFavoriteSvc() (*FavoriteService, *fakeFavoriteRepo, *fakeProductRepo) {
	favRepo := newFakeFavoriteRepo()
	prodRepo := newFakeProductRepo()
	svc := NewFavoriteService(favRepo, prodRepo, slog.Default())
	return svc, favRepo, prodRepo
}

func seedProduct(t *testing.T, repo *fakeProductRepo, sellerID uint, price float64, status string) *model.Product {
	t.Helper()
	p := &model.Product{
		SellerID: sellerID, Title: "测试商品", Price: price,
		Category: constants.ProductCategoryBooks, Condition: "九成新",
		Campus: "东校区", TradeLocation: "东门", Status: status,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return p
}

func TestFavoriteServiceAddGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("seller cannot favorite own product", func(t *testing.T) {
		svc, _, prodRepo := setupFavoriteSvc()
		p := seedProduct(t, prodRepo, 1, 100, constants.ProductStatusOnSale)
		if _, err := svc.Add(ctx, 1, p.ID); err == nil {
			t.Fatalf("expected forbidden error when seller favorites own product")
		}
	})

	t.Run("cannot favorite product not on sale", func(t *testing.T) {
		svc, _, prodRepo := setupFavoriteSvc()
		for _, status := range []string{constants.ProductStatusReserved, constants.ProductStatusSold, constants.ProductStatusRemoved} {
			p := seedProduct(t, prodRepo, 1, 100, status)
			if _, err := svc.Add(ctx, 2, p.ID); err == nil {
				t.Fatalf("expected conflict error for status %s", status)
			}
		}
	})

	t.Run("missing product returns not found", func(t *testing.T) {
		svc, _, _ := setupFavoriteSvc()
		if _, err := svc.Add(ctx, 2, 999); err == nil {
			t.Fatalf("expected not found error")
		}
	})

	t.Run("duplicate favorite keeps a single record", func(t *testing.T) {
		svc, favRepo, prodRepo := setupFavoriteSvc()
		p := seedProduct(t, prodRepo, 1, 100, constants.ProductStatusOnSale)
		first, err := svc.Add(ctx, 2, p.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		second, err := svc.Add(ctx, 2, p.ID)
		if err != nil {
			t.Fatalf("unexpected error on duplicate: %v", err)
		}
		if first.ID != second.ID {
			t.Fatalf("duplicate favorite created a new record: %d vs %d", first.ID, second.ID)
		}
		if favRepo.count() != 1 {
			t.Fatalf("expected 1 favorite record, got %d", favRepo.count())
		}
	})

	t.Run("favorite records price at favorite time", func(t *testing.T) {
		svc, _, prodRepo := setupFavoriteSvc()
		p := seedProduct(t, prodRepo, 1, 88.5, constants.ProductStatusOnSale)
		f, err := svc.Add(ctx, 2, p.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.PriceAtFavorite != 88.5 {
			t.Fatalf("expected price_at_favorite 88.5, got %v", f.PriceAtFavorite)
		}
	})
}

func TestFavoriteServiceAddConcurrent(t *testing.T) {
	ctx := context.Background()
	svc, favRepo, prodRepo := setupFavoriteSvc()
	p := seedProduct(t, prodRepo, 1, 100, constants.ProductStatusOnSale)

	const workers = 32
	var wg sync.WaitGroup
	errs := make([]error, workers)
	ids := make([]uint, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f, err := svc.Add(ctx, 2, p.ID)
			errs[i] = err
			if f != nil {
				ids[i] = f.ID
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent request %d failed: %v", i, err)
		}
	}
	if favRepo.count() != 1 {
		t.Fatalf("expected exactly 1 favorite record, got %d", favRepo.count())
	}
	if ids[0] == 0 {
		t.Fatalf("expected a non-zero favorite id")
	}
	for i, id := range ids {
		if id != ids[0] {
			t.Fatalf("request %d returned favorite id %d, want the shared id %d", i, id, ids[0])
		}
	}
}

func TestFavoriteServiceRemove(t *testing.T) {
	ctx := context.Background()
	svc, favRepo, prodRepo := setupFavoriteSvc()
	p := seedProduct(t, prodRepo, 1, 100, constants.ProductStatusOnSale)
	if _, err := svc.Add(ctx, 2, p.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := svc.Remove(ctx, 2, p.ID); err != nil {
		t.Fatalf("unexpected remove error: %v", err)
	}
	if favRepo.count() != 0 {
		t.Fatalf("expected favorite removed")
	}
	if err := svc.Remove(ctx, 2, p.ID); err == nil {
		t.Fatalf("expected not found error when removing twice")
	}
	// removing someone else's favorite must not affect my record
	if _, err := svc.Add(ctx, 2, p.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := svc.Remove(ctx, 3, p.ID); err == nil {
		t.Fatalf("expected not found when other user removes")
	}
	if favRepo.count() != 1 {
		t.Fatalf("other user's remove must not delete my favorite")
	}
}

// favoriteListRepo joins favorites with products for ListMine tests.
type favoriteListRepo struct {
	favorites []model.Favorite
	products  []model.Product
}

func (f *favoriteListRepo) Create(context.Context, *model.Favorite) error { return nil }
func (f *favoriteListRepo) FindByUserAndProduct(context.Context, uint, uint) (*model.Favorite, error) {
	return nil, util.ErrNotFound
}
func (f *favoriteListRepo) Delete(context.Context, uint, uint) error { return util.ErrNotFound }
func (f *favoriteListRepo) ListByUser(_ context.Context, userID uint, status string) ([]model.Favorite, []model.Product, error) {
	var favs []model.Favorite
	for _, fav := range f.favorites {
		if fav.UserID != userID {
			continue
		}
		if status != "" {
			var p *model.Product
			for i := range f.products {
				if f.products[i].ID == fav.ProductID {
					p = &f.products[i]
				}
			}
			if p == nil || p.Status != status {
				continue
			}
		}
		favs = append(favs, fav)
	}
	return favs, f.products, nil
}

func TestFavoriteServiceListMine(t *testing.T) {
	ctx := context.Background()
	prodRepo := newFakeProductRepo()
	repo := &favoriteListRepo{
		favorites: []model.Favorite{
			{ID: 1, UserID: 2, ProductID: 10, PriceAtFavorite: 100},
			{ID: 2, UserID: 2, ProductID: 11, PriceAtFavorite: 50},
			{ID: 3, UserID: 2, ProductID: 12, PriceAtFavorite: 80},
			{ID: 4, UserID: 3, ProductID: 10, PriceAtFavorite: 100},
		},
		products: []model.Product{
			{ID: 10, SellerID: 1, Title: "降价商品", Price: 60, Status: constants.ProductStatusOnSale},
			{ID: 11, SellerID: 1, Title: "涨价商品", Price: 70, Status: constants.ProductStatusOnSale},
			{ID: 12, SellerID: 1, Title: "已售出商品", Price: 80, Status: constants.ProductStatusSold},
		},
	}
	svc := NewFavoriteService(repo, prodRepo, slog.Default())

	t.Run("only own favorites with price drop flags", func(t *testing.T) {
		items, err := svc.ListMine(ctx, 2, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}
		for _, it := range items {
			if it.UserID != 2 {
				t.Fatalf("leaked other user's favorite")
			}
			switch it.ProductID {
			case 10:
				if !it.PriceDropped || it.CurrentPrice != 60 || it.PriceAtFavorite != 100 {
					t.Fatalf("expected price drop mark for product 10: %+v", it)
				}
			case 11:
				if it.PriceDropped {
					t.Fatalf("price increase must not be marked as drop")
				}
			case 12:
				if it.PriceDropped {
					t.Fatalf("unchanged price must not be marked as drop")
				}
				if it.Product == nil || it.Product.Status != constants.ProductStatusSold {
					t.Fatalf("expected sold product info")
				}
			}
		}
	})

	t.Run("filter by product status", func(t *testing.T) {
		items, err := svc.ListMine(ctx, 2, constants.ProductStatusSold)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 || items[0].ProductID != 12 {
			t.Fatalf("expected only the sold favorite, got %+v", items)
		}
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		if _, err := svc.ListMine(ctx, 2, "not_a_status"); err == nil {
			t.Fatalf("expected validation error for bad status")
		}
	})
}

