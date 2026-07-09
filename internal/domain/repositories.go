package domain

import (
	"context"
	"time"
)

type FeedItemRepository interface {
	Count(ctx context.Context) (int64, error)
	Create(ctx context.Context, item *FeedItem) error
	// BUG FIX / DIZAYN DƏYİŞİKLİYİ: title indi RSS-dən yox, scrape mərhələsindən
	// gəlir (bax fetcher.go və scraper_service.go) — ona görə UpdateScrapedData
	// da title-ı qəbul edib yazmalıdır.
	UpdateScrapedData(ctx context.Context, id int64, title, author, publishedDate, content, contentHTML, viewURL string, images []ImageItem, videoURL string) error
	GetAll(ctx context.Context, limit, offset int) ([]FeedItem, error)
	GetByID(ctx context.Context, id int64) (*FeedItem, error)
	GetBySource(ctx context.Context, sourceID int64, limit, offset int) ([]FeedItem, error)
	GetBySourceAfterScrapedAt(ctx context.Context, sourceID int64, after time.Time) ([]FeedItem, error)
	GetUnscraped(ctx context.Context, limit int) ([]FeedItem, error)
	GetEmptyContent(ctx context.Context, limit int) ([]FeedItem, error)
}

type SourceRepository interface {
	Create(ctx context.Context, s *Source) error
	GetAll(ctx context.Context) ([]Source, error)
	GetActive(ctx context.Context) ([]Source, error)
	GetByID(ctx context.Context, id int64) (*Source, error)
	UpdateLastPolled(ctx context.Context, id int64) error
	UpdateLastExportedAt(ctx context.Context, id int64) error
	IncrementFailCount(ctx context.Context, id int64) error
}
