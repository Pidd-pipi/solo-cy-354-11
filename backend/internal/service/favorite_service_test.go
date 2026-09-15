package service

import (
	"context"
	"errors"
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

// raceFallbackRepo simulates a request that always loses the insert race:
// the pre-insert lookup misses, its own insert is ignored by the unique
// index (model ID stays zero), and the rival's canonical row appears on the
// refetch. It also serves list queries for price-drop regression checks.
type raceFallbackRepo struct {
	mu       sync.Mutex
	row      *model.Favorite
	products []model.Product
	inserted bool
}

func (r *raceFallbackRepo) Create(_ context.Context, _ *model.Favorite) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inserted = true // rival row now exists; our insert was ignored
	return nil
}

func (r *raceFallbackRepo) FindByUserAndProduct(_ context.Context, userID, productID uint) (*model.Favorite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.inserted || r.row == nil || r.row.UserID != userID || r.row.ProductID != productID {
		return nil, util.ErrNotFound
	}
	cp := *r.row
	return &cp, nil
}

func (r *raceFallbackRepo) Delete(context.Context, uint, uint) error { return util.ErrNotFound }

func (r *raceFallbackRepo) ListByUser(_ context.Context, userID uint, _ string) ([]model.Favorite, []model.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.inserted || r.row == nil || r.row.UserID != userID {
		return nil, r.products, nil
	}
	return []model.Favorite{*r.row}, r.products, nil
}

// lostRowRepo ignores the insert but the refetch finds nothing.
type lostRowRepo struct{}

func (lostRowRepo) Create(context.Context, *model.Favorite) error { return nil }
func (lostRowRepo) FindByUserAndProduct(context.Context, uint, uint) (*model.Favorite, error) {
	return nil, util.ErrNotFound
}
func (lostRowRepo) Delete(context.Context, uint, uint) error { return util.ErrNotFound }
func (lostRowRepo) ListByUser(context.Context, uint, string) ([]model.Favorite, []model.Product, error) {
	return nil, nil, nil
}

// flakyCreateRepo fails the insert (e.g. duplicate entry) while the
// canonical row becomes visible only after the failed insert attempt.
type flakyCreateRepo struct {
	row      *model.Favorite
	inserted bool
}

func (r *flakyCreateRepo) Create(context.Context, *model.Favorite) error {
	r.inserted = true
	return errors.New("duplicate entry")
}
func (r *flakyCreateRepo) FindByUserAndProduct(_ context.Context, userID, productID uint) (*model.Favorite, error) {
	if !r.inserted || r.row == nil || r.row.UserID != userID || r.row.ProductID != productID {
		return nil, util.ErrNotFound
	}
	cp := *r.row
	return &cp, nil
}
func (r *flakyCreateRepo) Delete(context.Context, uint, uint) error { return util.ErrNotFound }
func (r *flakyCreateRepo) ListByUser(context.Context, uint, string) ([]model.Favorite, []model.Product, error) {
	return nil, nil, nil
}

// brokenCreateRepo fails the insert and nothing is found afterwards.
type brokenCreateRepo struct{}

func (brokenCreateRepo) Create(context.Context, *model.Favorite) error {
	return errors.New("connection reset")
}
func (brokenCreateRepo) FindByUserAndProduct(context.Context, uint, uint) (*model.Favorite, error) {
	return nil, util.ErrNotFound
}
func (brokenCreateRepo) Delete(context.Context, uint, uint) error { return util.ErrNotFound }
func (brokenCreateRepo) ListByUser(context.Context, uint, string) ([]model.Favorite, []model.Product, error) {
	return nil, nil, nil
}

func TestFavoriteServiceAddConflictFallback(t *testing.T) {
	ctx := context.Background()

	setupRace := func() (*FavoriteService, *raceFallbackRepo) {
		prodRepo := newFakeProductRepo()
		// The product now sells for 80, but the rival request favorited it at 100.
		p := &model.Product{
			SellerID: 1, Title: "测试商品", Price: 80,
			Category: constants.ProductCategoryBooks, Condition: "九成新",
			Campus: "东校区", TradeLocation: "东门", Status: constants.ProductStatusOnSale,
		}
		if err := prodRepo.Create(ctx, p); err != nil {
			t.Fatalf("seed product: %v", err)
		}
		repo := &raceFallbackRepo{
			row:      &model.Favorite{ID: 7, UserID: 2, ProductID: p.ID, PriceAtFavorite: 100},
			products: []model.Product{*p},
		}
		return NewFavoriteService(repo, prodRepo, slog.Default()), repo
	}

	t.Run("ignored insert returns the existing record", func(t *testing.T) {
		svc, _ := setupRace()
		f, err := svc.Add(ctx, 2, 1)
		if err != nil {
			t.Fatalf("conflict fallback must not fail: %v", err)
		}
		if f.ID != 7 {
			t.Fatalf("expected the canonical favorite id 7, got %d", f.ID)
		}
	})

	t.Run("first favorite price is not overwritten by the conflict", func(t *testing.T) {
		svc, _ := setupRace()
		f, err := svc.Add(ctx, 2, 1)
		if err != nil {
			t.Fatalf("conflict fallback must not fail: %v", err)
		}
		if f.PriceAtFavorite != 100 {
			t.Fatalf("price_at_favorite must stay 100 from the first favorite, got %v", f.PriceAtFavorite)
		}
	})

	t.Run("price drop reminder still computed after conflict fallback", func(t *testing.T) {
		svc, _ := setupRace()
		if _, err := svc.Add(ctx, 2, 1); err != nil {
			t.Fatalf("conflict fallback must not fail: %v", err)
		}
		items, err := svc.ListMine(ctx, 2, "")
		if err != nil {
			t.Fatalf("unexpected list error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 favorite item, got %d", len(items))
		}
		it := items[0]
		if !it.PriceDropped || it.PriceAtFavorite != 100 || it.CurrentPrice != 80 {
			t.Fatalf("expected price drop 100 -> 80, got %+v", it)
		}
	})

	t.Run("refetch miss after ignored insert returns a clear error", func(t *testing.T) {
		prodRepo := newFakeProductRepo()
		p := &model.Product{
			SellerID: 1, Title: "测试商品", Price: 80,
			Category: constants.ProductCategoryBooks, Condition: "九成新",
			Campus: "东校区", TradeLocation: "东门", Status: constants.ProductStatusOnSale,
		}
		if err := prodRepo.Create(ctx, p); err != nil {
			t.Fatalf("seed product: %v", err)
		}
		svc := NewFavoriteService(lostRowRepo{}, prodRepo, slog.Default())
		_, err := svc.Add(ctx, 2, p.ID)
		if err == nil {
			t.Fatalf("expected an error when the refetch finds no row")
		}
		var appErr *util.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Status != 500 || appErr.Code != constants.CodeInternalError {
			t.Fatalf("expected 500/internal error, got status=%d code=%d", appErr.Status, appErr.Code)
		}
	})

	t.Run("insert error returns the existing row when visible", func(t *testing.T) {
		prodRepo := newFakeProductRepo()
		p := &model.Product{
			SellerID: 1, Title: "测试商品", Price: 80,
			Category: constants.ProductCategoryBooks, Condition: "九成新",
			Campus: "东校区", TradeLocation: "东门", Status: constants.ProductStatusOnSale,
		}
		if err := prodRepo.Create(ctx, p); err != nil {
			t.Fatalf("seed product: %v", err)
		}
		repo := &flakyCreateRepo{row: &model.Favorite{ID: 9, UserID: 2, ProductID: p.ID, PriceAtFavorite: 100}}
		svc := NewFavoriteService(repo, prodRepo, slog.Default())
		f, err := svc.Add(ctx, 2, p.ID)
		if err != nil {
			t.Fatalf("duplicate insert error must fall back to the existing row: %v", err)
		}
		if f.ID != 9 || f.PriceAtFavorite != 100 {
			t.Fatalf("expected canonical row id=9 price=100, got %+v", f)
		}
	})

	t.Run("insert error without a visible row returns a clear error", func(t *testing.T) {
		prodRepo := newFakeProductRepo()
		p := &model.Product{
			SellerID: 1, Title: "测试商品", Price: 80,
			Category: constants.ProductCategoryBooks, Condition: "九成新",
			Campus: "东校区", TradeLocation: "东门", Status: constants.ProductStatusOnSale,
		}
		if err := prodRepo.Create(ctx, p); err != nil {
			t.Fatalf("seed product: %v", err)
		}
		svc := NewFavoriteService(brokenCreateRepo{}, prodRepo, slog.Default())
		_, err := svc.Add(ctx, 2, p.ID)
		if err == nil {
			t.Fatalf("expected an error when insert fails and no row exists")
		}
		var appErr *util.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Status != 500 || appErr.Code != constants.CodeInternalError {
			t.Fatalf("expected 500/internal error, got status=%d code=%d", appErr.Status, appErr.Code)
		}
	})
}

func TestFavoriteServiceGuardsNotRegressed(t *testing.T) {
	ctx := context.Background()

	t.Run("seller favorite own product stays forbidden and writes nothing", func(t *testing.T) {
		svc, favRepo, prodRepo := setupFavoriteSvc()
		p := seedProduct(t, prodRepo, 1, 100, constants.ProductStatusOnSale)
		_, err := svc.Add(ctx, 1, p.ID)
		var appErr *util.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Status != 403 || appErr.Code != constants.CodeForbidden {
			t.Fatalf("expected 403/forbidden, got status=%d code=%d", appErr.Status, appErr.Code)
		}
		if favRepo.count() != 0 {
			t.Fatalf("forbidden favorite must not create a record")
		}
	})

	t.Run("not-on-sale favorite stays conflict and writes nothing", func(t *testing.T) {
		svc, favRepo, prodRepo := setupFavoriteSvc()
		p := seedProduct(t, prodRepo, 1, 100, constants.ProductStatusSold)
		_, err := svc.Add(ctx, 2, p.ID)
		var appErr *util.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Status != 409 || appErr.Code != constants.CodeConflict {
			t.Fatalf("expected 409/conflict, got status=%d code=%d", appErr.Status, appErr.Code)
		}
		if favRepo.count() != 0 {
			t.Fatalf("conflict favorite must not create a record")
		}
	})
}
