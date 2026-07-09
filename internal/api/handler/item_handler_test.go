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
	item *domain.FeedItem
}

func (f *fakeRepoWithItem) Count(ctx context.Context) (int64, error)                { return 1, nil }
func (f *fakeRepoWithItem) Create(ctx context.Context, item *domain.FeedItem) error { return nil }
func (f *fakeRepoWithItem) UpdateScrapedData(ctx context.Context, id int64, title, author, publishedDate, content, contentHTML, viewURL string, images []domain.ImageItem, videoURL string) error {
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
