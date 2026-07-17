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
	UpdateScrapedData(ctx context.Context, id int64, title, author, publishedDate, content, contentHTML, viewURL string, images []ImageItem, videoURL string, cveIDs []string) error
	GetAll(ctx context.Context, limit, offset int) ([]FeedItem, error)
	GetByID(ctx context.Context, id int64) (*FeedItem, error)
	GetBySource(ctx context.Context, sourceID int64, limit, offset int) ([]FeedItem, error)
	GetBySourceAfterScrapedAt(ctx context.Context, sourceID int64, after time.Time) ([]FeedItem, error)
	GetUnscraped(ctx context.Context, limit int) ([]FeedItem, error)
	GetEmptyContent(ctx context.Context, limit int) ([]FeedItem, error)
	// GetRelatedByCVE — cveIDs-dən HƏR HANSI BİRİNİ paylaşan, AMMA
	// excludeID-dən fərqli olan məqalələri tapır (Postgres-in "&&" array
	// overlap operatoru ilə, bax feed_item_repository.go). Yüngül
	// RelatedFeedItem DTO qaytarır (tam content YOX) — bax cve.go və
	// migrations/011_cve_ids.sql.
	GetRelatedByCVE(ctx context.Context, cveIDs []string, excludeID int64, limit int) ([]RelatedFeedItem, error)
	// UpdateRelatedCVEFlags — cveIDs-dən HƏR HANSI BİRİNİ paylaşan bütün
	// item-lərin (KÖHNƏ + bu yeni scrape olunan, hamısı DAXİL) has_related_cve
	// sahəsini yenidən hesablayır. Scrape zamanı, bir item CVE ID-si ilə
	// yazılandan DƏRHAL SONRA çağırılmalıdır (bax scraper_service.go) —
	// əks halda, məsələn A məqaləsi bu gün, B məqaləsi 3 gün sonra eyni
	// CVE-ni yazsa, A-nın bayrağı köhnəlmiş (false) qalar, çünki A
	// scrape olunanda B hələ mövcud deyildi.
	UpdateRelatedCVEFlags(ctx context.Context, cveIDs []string) error
	// GetCVESummary — YALNIZ 2+ məqalədə keçən (yəni həqiqətən əlaqəli
	// olan) bütün CVE-ləri, hər birinin məqalə siyahısı ilə birlikdə
	// qaytarır. GET /api/v1/cves endpoint-i üçün — "hansı hadisələr
	// birdən çox mənbədə yazılıb" sualına birbaşa cavab (bax
	// GetRelatedByCVE-dən fərqli olaraq, bu, KONKRET bir item ID-si
	// tələb etmir — tam "kəşf" görünüşüdür).
	GetCVESummary(ctx context.Context) ([]CVESummary, error)
	// GetFieldEmptyStats — verilmiş mənbənin SON `limit` scrape olunmuş
	// item-i arasında hər sahənin (title/author/date/content) neçəsinin
	// BOŞ qaldığını sayır. Bu, "selector kövrəkliyi" görünürlüyü üçündür:
	// bir mənbə tam uğursuz olmasa da (fail_count artmır, çünki item-lər
	// texniki olaraq "uğurla" scrape olunur), sayt HTML strukturunu
	// dəyişəndə tək bir selector (məs. yalnız author) səssizcə sınıb boş
	// qala bilər — bax scraper_service.go-dakı checkFieldHealth.
	GetFieldEmptyStats(ctx context.Context, sourceID int64, limit int) (FieldEmptyStats, error)
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
