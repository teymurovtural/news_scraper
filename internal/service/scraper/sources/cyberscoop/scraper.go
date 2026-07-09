package cyberscoop

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"

	"example.com/new-scraper/internal/domain"
	"example.com/new-scraper/internal/service/scraper"
	"example.com/new-scraper/internal/service/scraper/base"

	"github.com/playwright-community/playwright-go"
)

const actionTimeout = float64(5000)

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

// ScrapePage — yalnız CyberScoop-a xas selector məntiqidir.
func (s *Scraper) ScrapePage(page playwright.Page, fi domain.FeedItem) scraper.ScrapeResult {
	title, err := page.Locator("h1.single-article__title").InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("cyberscoop: title alınmadı", "link", fi.Link, "error", err)
	}
	title = strings.TrimSpace(title)

	// Author — CSS text-transform:uppercase effektini aradan qaldırmaq üçün
	// strings.Title istifadə edirik (məs. "TIM STARKS" → "Tim Starks")
	authorRaw, err := page.Locator("span.single-article__author-names a[rel='author']").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("cyberscoop: author alınmadı", "link", fi.Link, "error", err)
	}
	author := toTitleCase(strings.TrimSpace(authorRaw))

	date, err := page.Locator("time").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("cyberscoop: tarix alınmadı", "link", fi.Link, "error", err)
	}
	date = strings.TrimSpace(date)

	// content_html — excerpt + content-inner birlikdə
	var htmlParts []string

	excerptHTML, err := page.Locator("div.single-article__excerpt").InnerHTML(playwright.LocatorInnerHTMLOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err == nil && strings.TrimSpace(excerptHTML) != "" {
		htmlParts = append(htmlParts, "<p><strong>"+strings.TrimSpace(excerptHTML)+"</strong></p>")
	}

	contentHTML, err := page.Locator("div.single-article__content-inner").InnerHTML(playwright.LocatorInnerHTMLOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("cyberscoop: content HTML alınmadı", "link", fi.Link, "error", err)
	} else {
		htmlParts = append(htmlParts, contentHTML)
	}

	rawCombined := strings.Join(htmlParts, "\n")

	removeSelectors := []string{
		"div.inline-ad",
		"div[class*='ad-']",
		"div.newsletter-signup",
		"div.related-articles",
	}

	cleanedHTML, err := base.CleanArticleHTML(rawCombined, removeSelectors)
	if err != nil {
		slog.Warn("cyberscoop: content HTML təmizlənmədi", "link", fi.Link, "error", err)
		cleanedHTML = rawCombined
	}

	content, err := base.HTMLToPlainText(cleanedHTML)
	if err != nil {
		slog.Warn("cyberscoop: plain text çıxarılmadı", "link", fi.Link, "error", err)
	}

	// Featured image
	var images []scraper.ImageItem
	featuredImg := page.Locator("figure.single-article__cover img.single-article__cover-image").First()
	src, err := featuredImg.GetAttribute("src", playwright.LocatorGetAttributeOptions{Timeout: playwright.Float(actionTimeout)})
	if err == nil && src != "" && !strings.HasPrefix(src, "data:image") {
		// BUG FIX: src bəzən nisbi yol ("/wp-content/uploads/...") ola bilir —
		// page.URL()-ə görə mütləq URL-ə çeviririk ki, DB-yə/export-a düzgün,
		// hər yerdə açıla bilən link yazılsın (bax base.ResolveURL şərhi).
		src = base.ResolveURL(page.URL(), src)

		alt, _ := featuredImg.GetAttribute("alt", playwright.LocatorGetAttributeOptions{Timeout: playwright.Float(actionTimeout)})
		images = append(images, scraper.ImageItem{URL: src, Alt: alt})

		// BUG FIX: src/alt HTML-escape olunmadan atributa yerləşdirilirdi —
		// alt mətnində " simvolu olsa strukturu poza bilərdi. html.EscapeString
		// ilə təhlükəsiz edirik.
		cleanedHTML = fmt.Sprintf(`<img src="%s" alt="%s"><br>`, html.EscapeString(src), html.EscapeString(alt)) + cleanedHTML
	}

	return scraper.ScrapeResult{
		Item: fi,
		Content: &scraper.ScrapedContent{
			Title:       title,
			Author:      author,
			Date:        date,
			Content:     content,
			ContentHTML: cleanedHTML,
			Images:      images,
		},
	}
}

// toTitleCase — "TIM STARKS" → "Tim Starks"
// CSS text-transform:uppercase render effektini normalize edir.
func toTitleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
