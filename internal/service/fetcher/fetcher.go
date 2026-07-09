package fetcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"example.com/new-scraper/internal/domain"
	"example.com/new-scraper/internal/platform/netguard"

	"github.com/mmcdole/gofeed"
)

// safeHTTPClient — gofeed-in RSS fetch etmək üçün istifadə etdiyi HTTP
// client. Adi http.DefaultClient əvəzinə BUNU istifadə etməyimizin səbəbi:
// Transport.DialContext netguard.SafeDialContext-ə işarə edir, yəni hər
// TCP bağlantısı açılmazdan ƏVVƏL, faktiki qoşulacaq IP-nin daxili/private
// olmadığı YENİDƏN yoxlanılır.
//
// TƏHLÜKƏSİZLİK QEYDİ (niyə bu, YALNIZ source_handler.validatePublicHTTPURL
// kifayət ETMİR): o yoxlama bir dəfə, mənbə YARADILAN anda edilir. Bu
// FetchSource isə HƏR mənbə üçün 15 dəqiqədən bir (scheduler tərəfindən)
// TƏKRAR çağırılır — hər çağırışda gofeed ÖZ DNS lookup-unu edir, yaradılma
// anındakı yoxlamadan tamamilə xəbərsiz. Domenin DNS-i yaradılmadan sonra
// dəyişə bilər (DNS rebinding) — client-in Transport-u olmasa, fetch anında
// heç bir qoruma qalmazdı. Ətraflı izah: internal/platform/netguard.
//
// Paket-səviyyəli dəyişən olaraq bir dəfə qurulur (hər FetchSource çağırışında
// yeni Transport yaratmaq lazımsız, bağlantı pool-unu da sıfırlayardı).
var safeHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext: netguard.SafeDialContext,
	},
}

type FetcherService struct {
	sourceRepo   domain.SourceRepository
	feedItemRepo domain.FeedItemRepository
}

func NewFetcherService(
	sourceRepo domain.SourceRepository,
	feedItemRepo domain.FeedItemRepository,
) *FetcherService {
	return &FetcherService{
		sourceRepo:   sourceRepo,
		feedItemRepo: feedItemRepo,
	}
}

func (s *FetcherService) FetchAll(ctx context.Context) error {
	sources, err := s.sourceRepo.GetActive(ctx)
	if err != nil {
		return fmt.Errorf("fetcher: aktiv mənbələr alınmadı: %w", err)
	}

	for _, source := range sources {
		if err := s.FetchSource(ctx, source); err != nil {
			slog.Error("fetcher: mənbə çəkilmədi", "source", source.Name, "feed_url", source.FeedURL, "error", err)
			s.sourceRepo.IncrementFailCount(ctx, source.ID)
			continue
		}
	}

	return nil
}

func (s *FetcherService) FetchSource(ctx context.Context, source domain.Source) error {
	fp := gofeed.NewParser()
	fp.Client = safeHTTPClient

	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	feed, err := fp.ParseURLWithContext(source.FeedURL, fetchCtx)
	if err != nil {
		return fmt.Errorf("fetcher: feed parse edilmədi [%s]: %w", source.FeedURL, err)
	}

	newItems := 0
	for _, item := range feed.Items {
		feedItem := mapToFeedItem(item, source.ID)

		err := s.feedItemRepo.Create(ctx, feedItem)
		if errors.Is(err, domain.ErrDuplicateItem) {
			continue
		}
		if err != nil {
			slog.Error("fetcher: xəbər yazılmadı", "link", item.Link, "error", err)
			continue
		}

		newItems++
	}

	if err := s.sourceRepo.UpdateLastPolled(ctx, source.ID); err != nil {
		slog.Error("fetcher: last_polled_at yenilənmədi", "source_id", source.ID, "error", err)
	}

	slog.Info("fetcher: mənbə çəkildi", "source", source.Name, "new_items", newItems)

	return nil
}

// mapToFeedItem — RSS-dən YALNIZ link (+ source_id, published_at) götürür.
//
// DIZAYN DƏYİŞİKLİYİ: əvvəllər `Title: item.Title` RSS-dən birbaşa DB-yə
// yazılırdı. İndi title scrape mərhələsində (səhifənin öz H1-indən) alınır və
// `UpdateScrapedData`-da yazılır — RSS başlığı ümumiyyətlə istifadə olunmur.
// Bu, RSS-in bəzən qısaltdığı/fərqli formatladığı başlıq əvəzinə, məqalənin
// öz saytındakı əsl başlığını əsas götürmək üçündür. Bu o deməkdir ki, item
// scrape olunana qədər `title` boş qalır (DB-də `title TEXT NOT NULL`
// olduğu üçün boş sətir "" kimi saxlanılır, NULL yox).
func mapToFeedItem(item *gofeed.Item, sourceID int64) *domain.FeedItem {
	feedItem := &domain.FeedItem{
		SourceID: sourceID,
		Link:     item.Link,
	}

	if item.PublishedParsed != nil {
		t := *item.PublishedParsed
		feedItem.PublishedAt = &t
	} else if item.UpdatedParsed != nil {
		t := *item.UpdatedParsed
		feedItem.PublishedAt = &t
	}

	return feedItem
}
