package itsecurityguru

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

func (s *Scraper) ScrapePage(page playwright.Page, fi domain.FeedItem) scraper.ScrapeResult {
	title, err := page.Locator("h1.jeg_post_title").InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("itsecurityguru: title alınmadı", "link", fi.Link, "error", err)
	}
	title = strings.TrimSpace(title)

	// Author — əvvəlcə DOM, boşdursa meta fallback
	author, err := page.Locator("div.jeg_meta_author a").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil || strings.TrimSpace(author) == "" {
		if err != nil {
			slog.Warn("itsecurityguru: author DOM-dan alınmadı, meta fallback", "link", fi.Link, "error", err)
		}
		metaAuthor, mErr := page.Locator("meta[name='author']").GetAttribute("content", playwright.LocatorGetAttributeOptions{
			Timeout: playwright.Float(actionTimeout),
		})
		if mErr == nil && metaAuthor != "" {
			author = metaAuthor
		}
	}
	author = strings.TrimSpace(author)

	// Date — meta tag (ISO, ən etibarlı), DOM fallback
	date, err := page.Locator("meta[property='article:published_time']").GetAttribute("content", playwright.LocatorGetAttributeOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil || date == "" {
		domDate, dErr := page.Locator("div.jeg_meta_date a").First().InnerText(playwright.LocatorInnerTextOptions{
			Timeout: playwright.Float(actionTimeout),
		})
		if dErr != nil {
			slog.Warn("itsecurityguru: tarix alınmadı", "link", fi.Link, "error", dErr)
		}
		date = domDate
	}
	date = strings.TrimSpace(date)

	// content_html — content-inner konteyneri
	rawContentHTML, err := page.Locator("div.content-inner").First().InnerHTML(playwright.LocatorInnerHTMLOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("itsecurityguru: content HTML alınmadı", "link", fi.Link, "error", err)
	}

	removeSelectors := []string{
		"div.sharedaddy",      // paylaşma düymələri
		"div.jp-relatedposts", // related posts
		"div[id^='ad_']",
		"ins.adsbygoogle",
		".jeg_post_tags",
	}

	contentHTML, err := base.CleanArticleHTML(rawContentHTML, removeSelectors)
	if err != nil {
		slog.Warn("itsecurityguru: content HTML təmizlənmədi", "link", fi.Link, "error", err)
		contentHTML = rawContentHTML
	}

	content, err := base.HTMLToPlainText(contentHTML)
	if err != nil {
		slog.Warn("itsecurityguru: plain text çıxarılmadı", "link", fi.Link, "error", err)
	}

	// Featured image — lazy load üçün data-src öncelikli
	// (Ortaq lazy-load məntiqi base paketindədir.)
	var images []scraper.ImageItem
	featuredImg := page.Locator("div.jeg_featured img").First()
	src, alt := base.ExtractLazyImageAttr(featuredImg, actionTimeout)
	if src != "" {
		images = append(images, scraper.ImageItem{URL: src, Alt: alt})
	}

	// Content içindəki şəkillər
	contentImgs, _ := page.Locator("div.content-inner img").All()
	for _, img := range contentImgs {
		imgSrc, imgAlt := base.ExtractLazyImageAttr(img, actionTimeout)
		if imgSrc == "" {
			continue
		}
		images = append(images, scraper.ImageItem{URL: imgSrc, Alt: imgAlt})
	}

	// Featured image-i HTML-in əvvəlinə prepend et (content-inner-dən kənardadır)
	// BUG FIX: src/alt HTML-escape olunmadan atributa yerləşdirilirdi.
	if src != "" {
		contentHTML = fmt.Sprintf(`<img src="%s" alt="%s"><br>`, html.EscapeString(src), html.EscapeString(alt)) + contentHTML
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
		},
	}
}
