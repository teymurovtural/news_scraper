package scraper

import (
	"context"

	"example.com/new-scraper/internal/domain"
)

type ImageItem struct {
	URL     string
	Alt     string
	Caption string
}

type ScrapedContent struct {
	Title       string
	Author      string
	Date        string
	Content     string // plain text — axtarış, keyword matching (CVE, tag) üçün
	ContentHTML string // təmizlənmiş HTML — struktur (paraqraf sırası, başlıqlar, şəkil yerləri) qorunur, göstərmək üçün
	Images      []ImageItem
	VideoURL    string
}

type ScrapeResult struct {
	Item    domain.FeedItem
	Content *ScrapedContent
	Err     error
}

type Scraper interface {
	Scrape(ctx context.Context, link string) (*ScrapedContent, error)
	ScrapeWithTimeout(ctx context.Context, link string, timeoutMs int) (*ScrapedContent, error)
	ScrapeMultiple(ctx context.Context, items []domain.FeedItem, timeoutMs int) []ScrapeResult
}
