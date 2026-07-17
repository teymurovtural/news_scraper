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
	// IncrementFailCount — fail_count-u 1 artırır. 20-yə çatsa, mənbə
	// avtomatik is_active=false olur (bax source_repository.go). Qaytardığı
	// bool DƏQİQ BU ÇAĞIRIŞ mənbəni deaktiv ETDİYİNİ bildirir (yəni əvvəl
	// aktiv idi, indi deaktiv oldu) — çağıran tərəf bunu bir dəfəlik
	// XƏBƏRDARLIQ loglamaq üçün istifadə edir, hər fail-də yox.
	IncrementFailCount(ctx context.Context, id int64) (deactivated bool, err error)
	// ResetFailCount — fail_count-u sıfırlayır. RSS-fetch UĞURU ARTIQ bunu
	// avtomatik etmir (bax UpdateLastPolled şərhi) — bu, YALNIZ real content
	// axını təsdiqlənəndə (məs. bir item uğurla scrape olunanda) çağırılır.
	ResetFailCount(ctx context.Context, id int64) error
	// Deactivate — mənbəni "soft delete" edir: sətir DB-də qalır (tarixi
	// data, ona aid feed_items itmir), yalnız is_active=false olur və
	// artıq fetcher/scraper tərəfindən poll olunmur (bax GetActive).
	// Bilərəkdən HEÇ BİR HARD DELETE (SQL DELETE) metodu yoxdur — SOC/
	// təhlükəsizlik məlumat toplayan bir alətdə tarixi qeydlərin
	// itməməsi vacibdir.
	Deactivate(ctx context.Context, id int64) error
	// Activate — Deactivate-in əksi: is_active=true edir VƏ fail_count-u
	// sıfırlayır (təzə başlanğıc). Həm əl ilə (əvvəllər DELETE edilmiş bir
	// mənbəni geri qaytarmaq) həm də IncrementFailCount-un 20-limitinə görə
	// AVTOMATİK deaktiv olmuş bir mənbəni yenidən aktivləşdirmək üçün istifadə
	// olunur. Sətir tapılmasa ErrSourceNotFound qaytarır.
	Activate(ctx context.Context, id int64) error
}
