package domain

import "time"

type ImageItem struct {
	URL string `json:"url"`
	Alt string `json:"alt,omitempty"`
}

type FeedItem struct {
	ID            int64       `json:"id"`
	SourceID      int64       `json:"source_id"`
	Title         string      `json:"title"`
	Link          string      `json:"link"`
	Author        string      `json:"author"`
	PublishedDate string      `json:"published_date"`
	Content       string      `json:"content"`
	ContentHTML   string      `json:"content_html,omitempty"`
	ViewURL       string      `json:"view_url,omitempty"`
	Images        []ImageItem `json:"images"`
	VideoURL      string      `json:"video_url,omitempty"`
	// CVEIDs — məqalə mətnindən çıxarılan CVE ID-ləri (məs.
	// ["CVE-2026-58644"]). Eyni CVE-ni paylaşan məqalələri
	// əlaqələndirmək üçün istifadə olunur (bax
	// internal/service/scraper/cve.go).
	CVEIDs []string `json:"cve_ids,omitempty"`
	// HasRelatedCVE — bu item-in CVE ID-lərindən HƏR HANSI BİRİ başqa bir
	// item-də də varmı? (hesablanmış/keşlənmiş sahə, bax
	// migrations/012_has_related_cve.sql). Siyahı görünüşündə (GET
	// /api/v1/items) əlavə sorğu etmədən "bu xəbər başqa yerdə də
	// yazılıb" siqnalı vermək üçündür — dəqiq siyahını görmək üçün
	// GET /api/v1/items/{id} (related_items) və ya GET /api/v1/cves
	// istifadə olunur.
	HasRelatedCVE bool       `json:"has_related_cve"`
	IsScraped     bool       `json:"is_scraped"`
	PublishedAt   *time.Time `json:"published_at"`
	FetchedAt     time.Time  `json:"fetched_at"`
	ScrapedAt     *time.Time `json:"scraped_at"`
}

// RelatedFeedItem — "əlaqəli məqalə" siyahısı üçün YÜNGÜL DTO (tam
// FeedItem yox — content/images/HTML DAXİL EDİLMİR). Eyni CVE-ni paylaşan
// başqa mənbələrin məqalələrini göstərmək üçün istifadə olunur (bax
// FeedItemRepository.GetRelatedByCVE). Bilərəkdən kiçikdir ki,
// GET /api/v1/items/{id} cavabı lazımsız yerə şişməsin.
type RelatedFeedItem struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	SourceName string `json:"source_name"`
	Link       string `json:"link"`
}

// CVESummary — GET /api/v1/cves cavabının bir elementi: bir CVE ID-si və
// onu paylaşan (2+) bütün məqalələr. Yalnız HƏQİQƏTƏN 2+ mənbədə/məqalədə
// keçən CVE-lər qaytarılır (bax FeedItemRepository.GetCVESummary) — tək
// məqalədə keçən CVE-lər burda görünmür, çünki "əlaqələndirmə" mənasızdır.
type CVESummary struct {
	CVEID string            `json:"cve_id"`
	Count int               `json:"count"`
	Items []RelatedFeedItem `json:"items"`
}

// FieldEmptyStats — bir mənbənin son N scrape olunmuş item-i arasında hər
// sahənin neçəsinin boş qaldığının sayı (bax
// FeedItemRepository.GetFieldEmptyStats, scraper_service.go-dakı
// checkFieldHealth). Faiz deyil, xam say saxlanılır — nisbəti çağıran tərəf
// (Total-a bölərək) hesablayır, çünki Total=0 olanda (heç scrape olunmayıb)
// bölmə xətasının qarşısını almaq çağıran tərəfdə daha rahatdır.
type FieldEmptyStats struct {
	Total        int
	EmptyTitle   int
	EmptyAuthor  int
	EmptyDate    int
	EmptyContent int
}
