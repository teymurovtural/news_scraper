package router_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/new-scraper/internal/api/handler"
	"example.com/new-scraper/internal/api/router"
	"example.com/new-scraper/internal/domain"
)

// fakeFeedItemRepo — router+handler inteqrasiya testləri üçün minimal saxta
// FeedItemRepository. Yalnız GetAll/GetByID istifadə olunur, qalanları boş.
type fakeFeedItemRepo struct{}

func (f *fakeFeedItemRepo) Count(ctx context.Context) (int64, error)                { return 1, nil }
func (f *fakeFeedItemRepo) Create(ctx context.Context, item *domain.FeedItem) error { return nil }
func (f *fakeFeedItemRepo) UpdateScrapedData(ctx context.Context, id int64, title, author, publishedDate, content, contentHTML, viewURL string, images []domain.ImageItem, videoURL string) error {
	return nil
}
func (f *fakeFeedItemRepo) GetAll(ctx context.Context, limit, offset int) ([]domain.FeedItem, error) {
	return []domain.FeedItem{{ID: 1, Title: "test"}}, nil
}
func (f *fakeFeedItemRepo) GetByID(ctx context.Context, id int64) (*domain.FeedItem, error) {
	return &domain.FeedItem{ID: id, Title: "test item", ContentHTML: "<p>content</p>"}, nil
}
func (f *fakeFeedItemRepo) GetBySource(ctx context.Context, sourceID int64, limit, offset int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (f *fakeFeedItemRepo) GetBySourceAfterScrapedAt(ctx context.Context, sourceID int64, after time.Time) ([]domain.FeedItem, error) {
	return nil, nil
}
func (f *fakeFeedItemRepo) GetUnscraped(ctx context.Context, limit int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (f *fakeFeedItemRepo) GetEmptyContent(ctx context.Context, limit int) ([]domain.FeedItem, error) {
	return nil, nil
}

type fakeSourceRepo struct{}

func (f *fakeSourceRepo) Create(ctx context.Context, s *domain.Source) error { return nil }
func (f *fakeSourceRepo) GetAll(ctx context.Context) ([]domain.Source, error) {
	return []domain.Source{{ID: 1, Name: "test source"}}, nil
}
func (f *fakeSourceRepo) GetActive(ctx context.Context) ([]domain.Source, error) { return nil, nil }
func (f *fakeSourceRepo) GetByID(ctx context.Context, id int64) (*domain.Source, error) {
	return &domain.Source{ID: id, Name: "test source"}, nil
}
func (f *fakeSourceRepo) UpdateLastPolled(ctx context.Context, id int64) error     { return nil }
func (f *fakeSourceRepo) UpdateLastExportedAt(ctx context.Context, id int64) error { return nil }
func (f *fakeSourceRepo) IncrementFailCount(ctx context.Context, id int64) (bool, error) {
	return false, nil
}
func (f *fakeSourceRepo) ResetFailCount(ctx context.Context, id int64) error { return nil }
func (f *fakeSourceRepo) Deactivate(ctx context.Context, id int64) error     { return nil }
func (f *fakeSourceRepo) Activate(ctx context.Context, id int64) error       { return nil }

// fakePinger — HealthHandler testləri üçün saxta Pinger. Default olaraq
// həmişə uğurlu (DB "sağlamdır") davranır.
type fakePinger struct {
	err error
}

func (f *fakePinger) Ping(ctx context.Context) error { return f.err }

func newTestRouter(apiKey string) http.Handler {
	itemHandler := handler.NewItemHandler(&fakeFeedItemRepo{})
	sourceHandler := handler.NewSourceHandler(&fakeSourceRepo{})
	healthHandler := handler.NewHealthHandler(&fakePinger{})
	return router.NewRouter(itemHandler, sourceHandler, healthHandler, router.Config{APIKey: apiKey})
}

// TestView_BypassesAuth — söhbətdə tapılan bug-ın regressiya testidir:
// /view endpoint-i API_KEY təyin olunsa belə, header-siz açıla bilməlidir
// (brauzerdə birbaşa açılmaq üçün nəzərdə tutulub).
func TestView_BypassesAuth(t *testing.T) {
	r := newTestRouter("secret123")

	req := httptest.NewRequest("GET", "/api/v1/items/1/view", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/view header-siz açıla bilməlidir, alındı: %d, body: %s", rec.Code, rec.Body.String())
	}
}

// TestProtectedEndpoints_RequireAuth — /view xaricindəki bütün endpoint-lər
// API_KEY tələb etməlidir (auth-un tam açılmadığından əmin oluruq).
func TestProtectedEndpoints_RequireAuth(t *testing.T) {
	r := newTestRouter("secret123")

	paths := []string{
		"/api/v1/items",
		"/api/v1/items/1",
		"/api/v1/sources",
		"/api/v1/sources/1",
	}

	for _, path := range paths {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s header-siz 401 qaytarmalıdır, alındı: %d", path, rec.Code)
		}
	}
}

// TestProtectedEndpoints_WorkWithCorrectKey — doğru key ilə bu endpoint-lər
// işləməlidir (auth-un özü də düzgün işlədiyindən əmin oluruq).
func TestProtectedEndpoints_WorkWithCorrectKey(t *testing.T) {
	r := newTestRouter("secret123")

	req := httptest.NewRequest("GET", "/api/v1/items", nil)
	req.Header.Set("X-API-Key", "secret123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("doğru key ilə /items işləməlidir, alındı: %d", rec.Code)
	}
}

// TestDeleteSource_RequiresAuthAndWorksWithCorrectKey — yeni DELETE
// /api/v1/sources/{id} route-unun (soft delete) həm auth arxasında
// qorunduğunu, həm də düzgün key ilə işlədiyini təsdiqləyir.
func TestDeleteSource_RequiresAuthAndWorksWithCorrectKey(t *testing.T) {
	r := newTestRouter("secret123")

	// Header-siz — 401
	req1 := httptest.NewRequest("DELETE", "/api/v1/sources/1", nil)
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("DELETE /sources/1 header-siz 401 qaytarmalıdır, alındı: %d", rec1.Code)
	}

	// Düzgün key ilə — 204
	req2 := httptest.NewRequest("DELETE", "/api/v1/sources/1", nil)
	req2.Header.Set("X-API-Key", "secret123")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNoContent {
		t.Errorf("DELETE /sources/1 doğru key ilə 204 qaytarmalıdır, alındı: %d", rec2.Code)
	}
}

// TestActivateSource_RequiresAuthAndWorksWithCorrectKey — yeni POST
// /api/v1/sources/{id}/activate route-unun (Deactivate-in əksi — həm əl
// ilə, həm avtomatik deaktiv olmuş mənbələri geri qaytarmaq üçün) həm auth
// arxasında qorunduğunu, həm də düzgün key ilə işlədiyini təsdiqləyir.
func TestActivateSource_RequiresAuthAndWorksWithCorrectKey(t *testing.T) {
	r := newTestRouter("secret123")

	// Header-siz — 401
	req1 := httptest.NewRequest("POST", "/api/v1/sources/1/activate", nil)
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("POST /sources/1/activate header-siz 401 qaytarmalıdır, alındı: %d", rec1.Code)
	}

	// Düzgün key ilə — 204
	req2 := httptest.NewRequest("POST", "/api/v1/sources/1/activate", nil)
	req2.Header.Set("X-API-Key", "secret123")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNoContent {
		t.Errorf("POST /sources/1/activate doğru key ilə 204 qaytarmalıdır, alındı: %d", rec2.Code)
	}
}

// TestHealthz_BypassesAuthAndReportsDBStatus — /healthz-in (1) auth-suz
// açıldığını və (2) DB Ping-inin nəticəsinə görə düzgün status kodu
// qaytardığını təsdiqləyir.
func TestHealthz_BypassesAuthAndReportsDBStatus(t *testing.T) {
	// DB sağlamdır — 200, auth-suz da işləməlidir
	healthyHandler := handler.NewHealthHandler(&fakePinger{})
	r1 := router.NewRouter(
		handler.NewItemHandler(&fakeFeedItemRepo{}),
		handler.NewSourceHandler(&fakeSourceRepo{}),
		healthyHandler,
		router.Config{APIKey: "secret123"},
	)
	req1 := httptest.NewRequest("GET", "/healthz", nil)
	rec1 := httptest.NewRecorder()
	r1.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("/healthz (sağlam DB) 200 qaytarmalıdır, alındı: %d", rec1.Code)
	}

	// DB Ping xəta versə — 503
	unhealthyHandler := handler.NewHealthHandler(&fakePinger{err: errors.New("connection refused")})
	r2 := router.NewRouter(
		handler.NewItemHandler(&fakeFeedItemRepo{}),
		handler.NewSourceHandler(&fakeSourceRepo{}),
		unhealthyHandler,
		router.Config{APIKey: "secret123"},
	)
	req2 := httptest.NewRequest("GET", "/healthz", nil)
	rec2 := httptest.NewRecorder()
	r2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusServiceUnavailable {
		t.Errorf("/healthz (DB xətası) 503 qaytarmalıdır, alındı: %d", rec2.Code)
	}
}

// TestViewAndItemsByID_DoNotConflict — Go 1.22+ ServeMux-un "/items/{id}/view"
// pattern-ini "/items/{id}"-dən daha spesifik seçdiyini təsdiqləyir —
// /view-in auth-suz olması digər endpoint-lərin qorunmasını pozmamalıdır.
func TestViewAndItemsByID_DoNotConflict(t *testing.T) {
	r := newTestRouter("secret123")

	// /items/{id} header-siz — 401 olmalıdır (view ilə qarışmamalıdır)
	req1 := httptest.NewRequest("GET", "/api/v1/items/42", nil)
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("/items/42 qorunmalıdır, alındı: %d", rec1.Code)
	}

	// /items/{id}/view header-siz — 200 olmalıdır
	req2 := httptest.NewRequest("GET", "/api/v1/items/42/view", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("/items/42/view auth-suz açılmalıdır, alındı: %d", rec2.Code)
	}
}
