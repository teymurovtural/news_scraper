package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/new-scraper/internal/domain"
)

type fakeRepoWithItem struct {
	item    *domain.FeedItem
	related []domain.RelatedFeedItem // GetRelatedByCVE-nin qaytaracağı saxta nəticə
}

func (f *fakeRepoWithItem) Count(ctx context.Context) (int64, error)                { return 1, nil }
func (f *fakeRepoWithItem) Create(ctx context.Context, item *domain.FeedItem) error { return nil }
func (f *fakeRepoWithItem) UpdateScrapedData(ctx context.Context, id int64, title, author, publishedDate, content, contentHTML, viewURL string, images []domain.ImageItem, videoURL string, cveIDs []string) error {
	return nil
}
func (f *fakeRepoWithItem) GetAll(ctx context.Context, limit, offset int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (f *fakeRepoWithItem) GetByID(ctx context.Context, id int64) (*domain.FeedItem, error) {
	return f.item, nil
}
func (f *fakeRepoWithItem) GetBySource(ctx context.Context, sourceID int64, limit, offset int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (f *fakeRepoWithItem) GetBySourceAfterScrapedAt(ctx context.Context, sourceID int64, after time.Time) ([]domain.FeedItem, error) {
	return nil, nil
}
func (f *fakeRepoWithItem) GetUnscraped(ctx context.Context, limit int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (f *fakeRepoWithItem) GetEmptyContent(ctx context.Context, limit int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (f *fakeRepoWithItem) GetRelatedByCVE(ctx context.Context, cveIDs []string, excludeID int64, limit int) ([]domain.RelatedFeedItem, error) {
	return f.related, nil
}

// TestView_RendersContentHTMLVerbatim — DİZAYN DƏYİŞİKLİYİ: video embed
// artıq item_handler-də deyil, CleanArticleHTML-də (bax
// internal/service/scraper/base/htmlclean_test.go) idarə olunur — video
// scrape zamanı birbaşa ContentHTML-in daxilinə, öz DOM mövqeyində yazılır.
// item_handler.go artıq VideoURL-ə heç toxunmur, ContentHTML-i olduğu kimi
// göstərir. Bu test bunu təsdiqləyir: ContentHTML-in daxilində olan iframe
// (video daxil) dəyişmədən /view səhifəsinə düşməlidir.
func TestView_RendersContentHTMLVerbatim(t *testing.T) {
	item := &domain.FeedItem{
		ID:          1,
		Title:       "Test",
		ContentHTML: `<p>Əvvəlki mətn</p><iframe src="//www.youtube.com/embed/x5-NDM91Q7E"></iframe><p>Sonrakı mətn</p>`,
	}
	h := NewItemHandler(&fakeRepoWithItem{item: item})

	req := httptest.NewRequest("GET", "/api/v1/items/1/view", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	h.View(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, item.ContentHTML) {
		t.Errorf("ContentHTML olduğu kimi (dəyişmədən) render olunmalıdır, body: %s", body)
	}
}

// TestGetByID_IncludesRelatedItemsWhenCVEPresent — item-in CVEIDs sahəsi
// boş deyilsə, GetByID cavabına related_items daxil edilməlidir (bax
// GetRelatedByCVE, cve.go).
func TestGetByID_IncludesRelatedItemsWhenCVEPresent(t *testing.T) {
	item := &domain.FeedItem{
		ID:     1,
		Title:  "CISA Adds Exploited SharePoint RCE Zero-Day CVE-2026-58644 to KEV",
		CVEIDs: []string{"CVE-2026-58644"},
	}
	related := []domain.RelatedFeedItem{
		{ID: 54, Title: "Fresh SharePoint Vulnerability Exploited Soon After Disclosure", SourceName: "SecurityWeek", Link: "https://example.com/54"},
	}
	h := NewItemHandler(&fakeRepoWithItem{item: item, related: related})

	req := httptest.NewRequest("GET", "/api/v1/items/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"related_items"`) {
		t.Errorf("CVE-li item-də related_items sahəsi gözlənilir, body: %s", body)
	}
	if !strings.Contains(body, "SecurityWeek") {
		t.Errorf("related_items-in içində əlaqəli mənbənin adı gözlənilir, body: %s", body)
	}
}

// TestGetByID_OmitsRelatedItemsWhenNoCVE — item-in CVEIDs sahəsi boşdursa,
// GetRelatedByCVE HEÇ çağırılmamalıdır (lazımsız DB sorğusu), cavabda
// related_items sahəsi (omitempty sayəsində) görünməməlidir.
func TestGetByID_OmitsRelatedItemsWhenNoCVE(t *testing.T) {
	item := &domain.FeedItem{
		ID:    2,
		Title: "Risk Ledger Raises $32 Million in Series B Funding",
	}
	h := NewItemHandler(&fakeRepoWithItem{item: item})

	req := httptest.NewRequest("GET", "/api/v1/items/2", nil)
	req.SetPathValue("id", "2")
	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, `"related_items"`) {
		t.Errorf("CVE-siz item-də related_items sahəsi görünməməlidir, body: %s", body)
	}
}
