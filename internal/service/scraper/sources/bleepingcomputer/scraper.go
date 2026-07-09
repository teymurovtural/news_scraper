package bleepingcomputer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"example.com/new-scraper/internal/domain"
	"example.com/new-scraper/internal/service/scraper"
	"example.com/new-scraper/internal/service/scraper/base"

	"github.com/PuerkitoBio/goquery"
	"github.com/playwright-community/playwright-go"
)

const actionTimeout = float64(8000)

type Scraper struct {
	pw       *playwright.Playwright
	headless bool
}

func New(pw *playwright.Playwright, headless bool) *Scraper {
	return &Scraper{pw: pw, headless: headless}
}

func (s *Scraper) Scrape(ctx context.Context, link string) (*scraper.ScrapedContent, error) {
	return s.ScrapeWithTimeout(ctx, link, 30000)
}

func (s *Scraper) ScrapeWithTimeout(ctx context.Context, link string, timeoutMs int) (*scraper.ScrapedContent, error) {
	results := s.ScrapeMultiple(ctx, []domain.FeedItem{{Link: link}}, timeoutMs)
	if len(results) == 0 {
		return nil, fmt.Errorf("nəticə yoxdur")
	}
	return results[0].Content, results[0].Err
}

func (s *Scraper) ScrapeMultiple(ctx context.Context, items []domain.FeedItem, timeoutMs int) []scraper.ScrapeResult {
	return base.RunMultiple(s.pw, s.headless, items, timeoutMs, s)
}

// ScrapePage — yalnız BleepingComputer-a xas selector məntiqidir.
func (s *Scraper) ScrapePage(page playwright.Page, fi domain.FeedItem) scraper.ScrapeResult {
	// Related articles blokunı DOM-dan sil ki content_html-ə düşməsin
	if _, err := page.Evaluate(`() => {
		const el = document.querySelector('div.cz-related-article-wrapp');
		if (el) el.remove();
	}`); err != nil {
		slog.Warn("bleepingcomputer: related articles silinmədi", "link", fi.Link, "error", err)
	}

	title, err := page.Locator("div.article_section h1").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("bleepingcomputer: title alınmadı", "link", fi.Link, "error", err)
	}

	authorRaw, err := page.Locator("a[rel='author'] span[itemprop='author'], a[rel='author sponsored'] span[itemprop='author']").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("bleepingcomputer: author alınmadı", "link", fi.Link, "error", err)
	}
	author := strings.TrimSpace(authorRaw)

	date, err := page.Locator("li.cz-news-date").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("bleepingcomputer: tarix alınmadı", "link", fi.Link, "error", err)
	}

	// content_html — məqalənin tam HTML-i
	rawContentHTML, err := page.Locator("div.article_section").First().InnerHTML(playwright.LocatorInnerHTMLOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("bleepingcomputer: content HTML alınmadı", "link", fi.Link, "error", err)
	}

	// Lazy load şəkillərdə src placeholder gif-dir, əsl URL data-src-dadır.
	// goquery ilə data-src olan hər img-in src-ini data-src dəyəri ilə əvəz edirik.
	rawContentHTML = resolveLazyImages(rawContentHTML)

	// BUG FIX: title həm ayrıca (yuxarıda, "div.article_section h1" ilə) həm
	// də bu removeSelectors-a "h1" əlavə olunana qədər content HTML-in daxilində
	// (çünki div.article_section-ın TAM InnerHTML-i çəkilir, h1 də onun
	// içindədir) iki dəfə görünürdü — /view səhifəsində başlıq təkrarlanırdı.
	removeSelectors := []string{
		"h1",
		"div.cz-related-article-wrapp",
		"div.cz-news-share",
		"div[id^='ad_']",
		"div.adv-in",
		"div.newsletter-subscribe",
	}

	contentHTML, err := base.CleanArticleHTML(rawContentHTML, removeSelectors)
	if err != nil {
		slog.Warn("bleepingcomputer: content HTML təmizlənmədi", "link", fi.Link, "error", err)
		contentHTML = rawContentHTML
	}

	content, err := base.HTMLToPlainText(contentHTML)
	if err != nil {
		slog.Warn("bleepingcomputer: plain text çıxarılmadı", "link", fi.Link, "error", err)
	}

	// Şəkillər — yalnız bleepstatic.com-dan, məqaləyə aid olanlar.
	// b-lazy class-lı şəkillər lazy load işlədir: əsl URL data-src-dadır, src yox.
	var images []scraper.ImageItem
	imgLocators, err := page.Locator("div.article_section img").All()
	if err != nil {
		slog.Warn("bleepingcomputer: şəkillər alınmadı", "link", fi.Link, "error", err)
	} else {
		for _, img := range imgLocators {
			// Ortaq lazy-load məntiqi base paketindədir (data-src → src → data: URI rədd).
			src, alt := base.ExtractLazyImageAttr(img, actionTimeout)
			if src == "" {
				continue
			}
			if !strings.Contains(src, "bleepstatic.com") {
				continue
			}
			if !strings.Contains(src, "content/hl-images") && !strings.Contains(src, "images/news") {
				continue
			}
			images = append(images, scraper.ImageItem{URL: src, Alt: alt})
		}
	}

	// Bəzi məqalələrdə YouTube video embed olur (məs. demo/PoC videoları).
	// thehackernews-dəki eyni pattern: articlebody daxilində youtube.com/embed
	// src-li iframe axtarırıq, tapılsa onun src-ini VideoURL kimi saxlayırıq.
	videoURL := ""
	videoIframeLocators, err := page.Locator("div.article_section iframe[src*='youtube.com/embed']").All()
	if err != nil {
		slog.Warn("bleepingcomputer: iframe alınmadı", "link", fi.Link, "error", err)
	} else if len(videoIframeLocators) > 0 {
		src, _ := videoIframeLocators[0].GetAttribute("src", playwright.LocatorGetAttributeOptions{
			Timeout: playwright.Float(actionTimeout),
		})
		if src != "" {
			videoURL = src
			slog.Info("bleepingcomputer: video tapıldı", "link", fi.Link, "video_url", videoURL)
		}
	}

	return scraper.ScrapeResult{
		Item: fi,
		Content: &scraper.ScrapedContent{
			Title:       title,
			Author:      author,
			Date:        date,
			Content:     content,
			ContentHTML: contentHTML,
			Images:      images,
			VideoURL:    videoURL,
		},
	}
}

// resolveLazyImages — data-src olan img teqlərinin src-ini data-src dəyəri ilə əvəz edir.
// BleepingComputer b-lazy sistemi istifadə edir: src = 1px GIF placeholder, əsl URL = data-src.
func resolveLazyImages(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	doc.Find("img[data-src]").Each(func(_ int, s *goquery.Selection) {
		if dataSrc, exists := s.Attr("data-src"); exists && dataSrc != "" {
			s.SetAttr("src", dataSrc)
			s.RemoveAttr("data-src")
		}
	})

	result, err := doc.Find("body").First().Html()
	if err != nil {
		return html
	}
	return strings.TrimSpace(result)
}
