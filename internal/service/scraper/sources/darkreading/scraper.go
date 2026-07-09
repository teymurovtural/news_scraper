package darkreading

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

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

// ScrapePage — yalnız Dark Reading-ə xas selector məntiqidir.
func (s *Scraper) ScrapePage(page playwright.Page, fi domain.FeedItem) scraper.ScrapeResult {
	// Popup bağla
	popupTimeout := float64(5000)
	closeBtn := page.Locator("button.ub-emb-close")
	if err := closeBtn.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: &popupTimeout,
	}); err == nil {
		if err := closeBtn.Click(); err != nil {
			slog.Warn("darkreading: popup bağlanmadı", "link", fi.Link, "error", err)
		} else {
			slog.Info("darkreading: popup bağlandı", "link", fi.Link)
		}
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(1 * time.Second)

	title, err := page.Locator("h1.ArticleBase-HeaderTitle span.ArticleBase-LargeTitle").InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("darkreading: title alınmadı", "link", fi.Link, "error", err)
	}

	authorRaw, err := page.Locator("a.Contributors-ContributorName").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("darkreading: author alınmadı", "link", fi.Link, "error", err)
	}
	author := strings.TrimRight(strings.TrimSpace(authorRaw), ",")

	date, err := page.Locator("p.Contributors-Date").InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("darkreading: tarix alınmadı", "link", fi.Link, "error", err)
	}

	// content_html — məqalənin əsas content konteynerindən
	contentWrapper := page.Locator("div.TwoColumnLayout div[data-module='content']")
	rawContentHTML, err := contentWrapper.InnerHTML(playwright.LocatorInnerHTMLOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("darkreading: content HTML alınmadı", "link", fi.Link, "error", err)
	}

	removeSelectors := []string{
		"div[data-module='ad']",
		"div[data-module='newsletter']",
		"div[data-module='related']",
		"div.ContentSponsor",
	}

	contentHTML, err := base.CleanArticleHTML(rawContentHTML, removeSelectors)
	if err != nil {
		slog.Warn("darkreading: content HTML təmizlənmədi", "link", fi.Link, "error", err)
		contentHTML = rawContentHTML
	}

	content, err := base.HTMLToPlainText(contentHTML)
	if err != nil {
		slog.Warn("darkreading: plain text çıxarılmadı", "link", fi.Link, "error", err)
	}

	var images []scraper.ImageItem

	// Featured image
	featuredImg := page.Locator("img.ArticleBase-FeaturedImage").First()
	// BUG FIX: src/alt HTML-escape olunmadan atributa yerləşdirilirdi (digər
	// scraper-lərdə tapılan eyni class-lı bug — bax cyberscoop/securityweek/
	// itsecurityguru). Bu sayt hazırda deaktivdir, amma gələcəkdə aktivləşərsə
	// problemsiz olsun deyə düzəldilir.
	featuredSrc, alt := base.ExtractLazyImageAttr(featuredImg, actionTimeout)
	if featuredSrc != "" {
		images = append(images, scraper.ImageItem{URL: featuredSrc, Alt: alt})
		// Featured image content konteyneri xaricindədir — əvvələ prepend et
		contentHTML = fmt.Sprintf(`<img src="%s" alt="%s"><br>`, html.EscapeString(featuredSrc), html.EscapeString(alt)) + contentHTML
	}

	contentImgs, err := page.Locator("img[data-testid='content-image']").All()
	if err != nil {
		slog.Warn("darkreading: content şəkilləri alınmadı", "link", fi.Link, "error", err)
	} else {
		for _, img := range contentImgs {
			// Ortaq lazy-load məntiqi base paketindədir.
			src, imgAlt := base.ExtractLazyImageAttr(img, actionTimeout)
			if src == "" {
				continue
			}
			images = append(images, scraper.ImageItem{URL: src, Alt: imgAlt})
		}
	}

	videoURL := ""
	videoLink := page.Locator("a.ytmVideoInfoVideoTitle").First()
	href, err := videoLink.GetAttribute("href", playwright.LocatorGetAttributeOptions{Timeout: playwright.Float(actionTimeout)})
	if err == nil && href != "" {
		videoURL = href
		slog.Info("darkreading: video tapıldı", "link", fi.Link, "video_url", videoURL)
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
