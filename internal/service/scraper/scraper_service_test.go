package scraper

import (
	"context"
	"sync"
	"testing"
	"time"

	"example.com/new-scraper/internal/domain"
)

// fakeScraper — Scraper interfeysinin test üçün saxta implementasiyası.
// Hər ScrapeMultiple çağırışında ALDIĞI item-ləri yadda saxlayır ki,
// testdə "bu scraper-ə HANSI linklər verildi?" yoxlanıla bilsin.
type fakeScraper struct {
	mu    sync.Mutex
	name  string
	calls [][]domain.FeedItem
}

func (f *fakeScraper) Scrape(ctx context.Context, link string) (*ScrapedContent, error) {
	return &ScrapedContent{Title: f.name}, nil
}

func (f *fakeScraper) ScrapeWithTimeout(ctx context.Context, link string, timeoutMs int) (*ScrapedContent, error) {
	return &ScrapedContent{Title: f.name}, nil
}

func (f *fakeScraper) ScrapeMultiple(ctx context.Context, items []domain.FeedItem, timeoutMs int) []ScrapeResult {
	f.mu.Lock()
	f.calls = append(f.calls, items)
	f.mu.Unlock()

	results := make([]ScrapeResult, len(items))
	for i, it := range items {
		// Content.Title-a scraper-in öz adını yazırıq ki, testdə "bu item
		// HANSI scraper tərəfindən emal olundu?" yoxlanıla bilsin.
		results[i] = ScrapeResult{Item: it, Content: &ScrapedContent{Title: f.name}}
	}
	return results
}

// fakeFeedItemRepo — domain.FeedItemRepository-nin test üçün saxta
// implementasiyası. Yalnız UpdateScrapedData çağırışlarını yadda saxlayır,
// digər metodlar bu testlərdə istifadə olunmur.
type fakeFeedItemRepo struct {
	mu      sync.Mutex
	updates map[int64]string // item ID -> hansı scraper-in title-ı yazıldı
}

func newFakeFeedItemRepo() *fakeFeedItemRepo {
	return &fakeFeedItemRepo{updates: make(map[int64]string)}
}

func (r *fakeFeedItemRepo) UpdateScrapedData(ctx context.Context, id int64, title, author, publishedDate, content, contentHTML, viewURL string, images []domain.ImageItem, videoURL string, cveIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates[id] = title
	return nil
}

func (r *fakeFeedItemRepo) Count(ctx context.Context) (int64, error)                { return 0, nil }
func (r *fakeFeedItemRepo) Create(ctx context.Context, item *domain.FeedItem) error { return nil }
func (r *fakeFeedItemRepo) GetAll(ctx context.Context, limit, offset int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (r *fakeFeedItemRepo) GetByID(ctx context.Context, id int64) (*domain.FeedItem, error) {
	return nil, nil
}
func (r *fakeFeedItemRepo) GetBySource(ctx context.Context, sourceID int64, limit, offset int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (r *fakeFeedItemRepo) GetBySourceAfterScrapedAt(ctx context.Context, sourceID int64, after time.Time) ([]domain.FeedItem, error) {
	return nil, nil
}
func (r *fakeFeedItemRepo) GetUnscraped(ctx context.Context, limit int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (r *fakeFeedItemRepo) GetEmptyContent(ctx context.Context, limit int) ([]domain.FeedItem, error) {
	return nil, nil
}
func (r *fakeFeedItemRepo) GetRelatedByCVE(ctx context.Context, cveIDs []string, excludeID int64, limit int) ([]domain.RelatedFeedItem, error) {
	return nil, nil
}
func (r *fakeFeedItemRepo) UpdateRelatedCVEFlags(ctx context.Context, cveIDs []string) error {
	return nil
}
func (r *fakeFeedItemRepo) GetCVESummary(ctx context.Context) ([]domain.CVESummary, error) {
	return nil, nil
}

// fakeSourceRepo — domain.SourceRepository-nin bu testlər üçün saxta
// implementasiyası. Yalnız ScraperService-in kompilyasiya üçün buna ehtiyacı
// var (scrapeItems hər çağırışdan sonra sağlamlıq siqnalı göndərir) —
// aşağıdakı testlərin özü bunun davranışını yoxlamır.
type fakeSourceRepo struct{}

func (f *fakeSourceRepo) Create(ctx context.Context, s *domain.Source) error  { return nil }
func (f *fakeSourceRepo) GetAll(ctx context.Context) ([]domain.Source, error) { return nil, nil }
func (f *fakeSourceRepo) GetActive(ctx context.Context) ([]domain.Source, error) {
	return nil, nil
}
func (f *fakeSourceRepo) GetByID(ctx context.Context, id int64) (*domain.Source, error) {
	return nil, nil
}
func (f *fakeSourceRepo) UpdateLastPolled(ctx context.Context, id int64) error     { return nil }
func (f *fakeSourceRepo) UpdateLastExportedAt(ctx context.Context, id int64) error { return nil }
func (f *fakeSourceRepo) IncrementFailCount(ctx context.Context, id int64) (bool, error) {
	return false, nil
}
func (f *fakeSourceRepo) ResetFailCount(ctx context.Context, id int64) error { return nil }
func (f *fakeSourceRepo) Deactivate(ctx context.Context, id int64) error     { return nil }
func (f *fakeSourceRepo) Activate(ctx context.Context, id int64) error       { return nil }

// TestGroupAndChunk_DoesNotMixSources — bu, söhbətin ilk (və ən vacib)
// bug-ının regressiya testidir: mənbəyə görə qruplaşdırmadan əvvəl, qarışıq
// sıra ilə gələn item-lər (A,A,A,B,B,B,B,A,A kimi) 5-lik chunk-lara bölünəndə
// bəzi chunk-lar fərqli mənbələrdən ibarət ola bilirdi, nəticədə yanlış
// scraper-ə göndərilirdi. Bu test təsdiqləyir ki, HƏR chunk yalnız TƏK
// mənbədən ibarətdir.
func TestGroupAndChunk_DoesNotMixSources(t *testing.T) {
	scraperA := &fakeScraper{name: "A"}
	scraperB := &fakeScraper{name: "B"}

	scrapers := map[string]Scraper{
		"https://site-a.com": scraperA,
		"https://site-b.com": scraperB,
	}

	svc := NewScraperService(nil, &fakeSourceRepo{}, scrapers, 1, "")

	// Qarışıq sıra — real GetUnscraped() nəticəsini simulyasiya edir
	// (fərqli mənbələr eyni fetched_at civarında qarışıq sıraya düşə bilir).
	items := []domain.FeedItem{
		{ID: 1, Link: "https://site-a.com/1"},
		{ID: 2, Link: "https://site-a.com/2"},
		{ID: 3, Link: "https://site-a.com/3"},
		{ID: 4, Link: "https://site-b.com/1"},
		{ID: 5, Link: "https://site-b.com/2"},
		{ID: 6, Link: "https://site-b.com/3"},
		{ID: 7, Link: "https://site-b.com/4"},
		{ID: 8, Link: "https://site-a.com/4"},
		{ID: 9, Link: "https://site-a.com/5"},
	}

	chunks, unmatched := svc.groupAndChunk(items, 5)

	if len(unmatched) != 0 {
		t.Fatalf("gözlənilməz unmatched item-lər: %v", unmatched)
	}

	total := 0
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		total += len(chunk)

		// Bu chunk-ın bütün item-ləri EYNİ mənbədən olmalıdır.
		wantPrefix := prefixOf(chunk[0].Link)
		for _, item := range chunk {
			if prefixOf(item.Link) != wantPrefix {
				t.Errorf("QARIŞIQ CHUNK TAPILDI: %v (gözlənilən prefiks: %s)", chunk, wantPrefix)
			}
		}
	}

	if total != len(items) {
		t.Errorf("item itkisi: gözlənilən %d, alındı %d", len(items), total)
	}
}

func prefixOf(link string) string {
	if len(link) >= len("https://site-a.com") && link[:len("https://site-a.com")] == "https://site-a.com" {
		return "https://site-a.com"
	}
	return "https://site-b.com"
}

// TestScrapeItems_UsesCorrectScraperPerItem — groupAndChunk-dan bir addım
// irəli gedib, TAM AXINI (scrapeItems) test edir: hər item-in DB-yə yazılan
// nəticəsi (fakeFeedItemRepo.updates) məhz ONUN ÖZ mənbəyinin scraper-indən
// gəlməlidir, başqa mənbənin scraper-indən YOX. Bu, bug-ın "sükutla yanlış
// content yaradırdı" tərəfini tam əhatə edir.
func TestScrapeItems_UsesCorrectScraperPerItem(t *testing.T) {
	scraperA := &fakeScraper{name: "A"}
	scraperB := &fakeScraper{name: "B"}

	scrapers := map[string]Scraper{
		"https://site-a.com": scraperA,
		"https://site-b.com": scraperB,
	}

	repo := newFakeFeedItemRepo()
	svc := NewScraperService(repo, &fakeSourceRepo{}, scrapers, 2, "")

	items := []domain.FeedItem{
		{ID: 1, Link: "https://site-a.com/1"},
		{ID: 2, Link: "https://site-b.com/1"},
		{ID: 3, Link: "https://site-a.com/2"},
		{ID: 4, Link: "https://site-b.com/2"},
		{ID: 5, Link: "https://site-a.com/3"},
	}

	failed := svc.scrapeItems(context.Background(), items, 5000)

	if len(failed) != 0 {
		t.Fatalf("gözlənilməz uğursuzluqlar: %v", failed)
	}

	wantScraper := map[int64]string{
		1: "A", 2: "B", 3: "A", 4: "B", 5: "A",
	}

	for id, want := range wantScraper {
		got, ok := repo.updates[id]
		if !ok {
			t.Errorf("item %d DB-yə yazılmayıb", id)
			continue
		}
		if got != want {
			t.Errorf("item %d: YANLIŞ SCRAPER İSTİFADƏ OLUNUB — gözlənilən %q, alındı %q", id, want, got)
		}
	}
}
