package securityweek

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

// actionTimeout — InnerText/InnerHTML/GetAttribute kimi bütün DOM action-ları
// üçün vahid timeout. Bu dəyərdən çox gözləmək mənasızdır.
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

// ScrapePage — yalnız SecurityWeek-ə xas selector məntiqidir.
func (s *Scraper) ScrapePage(page playwright.Page, fi domain.FeedItem) scraper.ScrapeResult {
	// BUG FIX: page.Goto WaitUntilStateCommit ilə çağırılır (base.go) — yəni
	// server ilk cavab verən kimi davam edir, <head> hələ DOM-a əlavə
	// olunmamış ola bilər. AddStyleTag daxildə document.head.appendChild(...)
	// edir — head yoxdursa "Cannot read properties of null (reading
	// 'appendChild')" xətası verir. HTML-də <head> həmişə <body>-dən əvvəl
	// gəldiyi üçün, body-nin DOM-a əlavə olunduğunu gözləmək kifayətdir.
	if err := page.Locator("body").WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateAttached,
		Timeout: playwright.Float(actionTimeout),
	}); err != nil {
		slog.Warn("securityweek: body gözlənilmədi", "link", fi.Link, "error", err)
	}

	if _, err := page.AddStyleTag(playwright.PageAddStyleTagOptions{
		Content: playwright.String(`
			.pum-overlay, .pum-container { display: none !important; }
			body.pum-open { overflow: auto !important; }
		`),
	}); err != nil {
		slog.Warn("securityweek: popup CSS inject edilmədi", "link", fi.Link, "error", err)
	}

	time.Sleep(300 * time.Millisecond)

	hidePopup := func() {
		if _, err := page.Evaluate(`() => {
			document.querySelectorAll('.pum-overlay, .pum-container').forEach(el => {
				el.style.setProperty('display', 'none', 'important');
				el.classList.remove('pum-active');
			});
			document.body.classList.remove('pum-open');
			document.body.style.overflow = 'auto';
		}`); err != nil {
			slog.Warn("securityweek: popup gizlədilmədi", "link", fi.Link, "error", err)
		}
	}
	hidePopup()

	hidePopup()

	title, err := page.Locator("header.zox-post-head-wrap h1").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("securityweek: title alınmadı", "link", fi.Link, "error", err)
	}

	author, err := page.Locator("span.zox-author-name").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("securityweek: author alınmadı", "link", fi.Link, "error", err)
	}

	date, err := page.Locator("div.zox-post-date-wrap").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("securityweek: tarix alınmadı", "link", fi.Link, "error", err)
	}
	// "| July 2, 2026 (7:01 AM ET)" → "July 2, 2026 (7:01 AM ET)"
	date = strings.TrimSpace(strings.TrimLeft(date, "| "))

	// Excerpt — title-dan sonra gələn giriş paraqrafı (span.zox-post-excerpt > p)
	excerpt, _ := page.Locator("span.zox-post-excerpt p").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	excerpt = strings.TrimSpace(excerpt)

	// Scroll to bottom — lazy load content və reklamları trigger etmək üçün.
	// SecurityWeek reklam bloklarını yalnız scroll edəndə DOM-a əlavə edir.
	if _, err := page.Evaluate(`() => window.scrollTo(0, document.body.scrollHeight)`); err != nil {
		slog.Warn("securityweek: scroll xətası", "link", fi.Link, "error", err)
	}
	time.Sleep(1 * time.Second)
	if _, err := page.Evaluate(`() => window.scrollTo(0, 0)`); err != nil {
		slog.Warn("securityweek: scroll geri xətası", "link", fi.Link, "error", err)
	}
	time.Sleep(300 * time.Millisecond)

	// zox-post-body-nin tam yüklənməsini gözlə (WaitUntilStateCommit ilə DOM hazır olmaya bilər)
	if err := page.Locator("div.zox-post-body p").First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateAttached,
		Timeout: playwright.Float(actionTimeout),
	}); err != nil {
		slog.Warn("securityweek: content body gözlənilmədi", "link", fi.Link, "error", err)
	}

	// Reklam blokları content-dən sonra dinamik JS ilə əlavə olunur.
	// Scroll zamanı artıq DOM-a düşüblər — indi silirik.
	if _, err := page.Evaluate(`() => {
		[
			'div.zox-post-ad-wrap',
			'div[id^="placement_"]',
			'div.zox-related-posts-wrap',
			'div.zox-post-tags-wrap',
		].forEach(sel => {
			document.querySelectorAll(sel).forEach(el => el.remove());
		});
	}`); err != nil {
		slog.Warn("securityweek: reklam blokları silinmədi", "link", fi.Link, "error", err)
	}

	rawContentHTML, err := page.Locator("div.zox-post-body").First().InnerHTML(playwright.LocatorInnerHTMLOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	if err != nil {
		slog.Warn("securityweek: content HTML alınmadı", "link", fi.Link, "error", err)
	}

	removeSelectors := []string{
		"div.zox-post-ad-wrap",
		"div[id^='placement_']",
	}

	contentHTML, err := base.CleanArticleHTML(rawContentHTML, removeSelectors)
	if err != nil {
		slog.Warn("securityweek: content HTML təmizlənmədi", "link", fi.Link, "error", err)
		contentHTML = rawContentHTML
	}

	content, err := base.HTMLToPlainText(contentHTML)
	if err != nil {
		slog.Warn("securityweek: plain text çıxarılmadı", "link", fi.Link, "error", err)
	}

	var images []scraper.ImageItem
	featuredImg := page.Locator("div.zox-post-img img").First()
	featuredSrc, _ := featuredImg.GetAttribute("src", playwright.LocatorGetAttributeOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	featuredDataSrc, _ := featuredImg.GetAttribute("data-src", playwright.LocatorGetAttributeOptions{
		Timeout: playwright.Float(actionTimeout),
	})
	realFeaturedSrc := featuredDataSrc
	if realFeaturedSrc == "" {
		realFeaturedSrc = featuredSrc
	}
	if realFeaturedSrc != "" && !strings.HasPrefix(realFeaturedSrc, "data:image") {
		alt, _ := featuredImg.GetAttribute("alt", playwright.LocatorGetAttributeOptions{
			Timeout: playwright.Float(actionTimeout),
		})
		images = append(images, scraper.ImageItem{URL: realFeaturedSrc, Alt: alt})
	}

	contentImgLocators, err := page.Locator("div.zox-post-body img").All()
	if err != nil {
		slog.Warn("securityweek: content şəkilləri alınmadı", "link", fi.Link, "error", err)
	} else {
		for _, img := range contentImgLocators {
			src, _ := img.GetAttribute("src", playwright.LocatorGetAttributeOptions{
				Timeout: playwright.Float(actionTimeout),
			})
			dataSrc, _ := img.GetAttribute("data-src", playwright.LocatorGetAttributeOptions{
				Timeout: playwright.Float(actionTimeout),
			})
			realSrc := dataSrc
			if realSrc == "" {
				realSrc = src
			}
			if realSrc == "" || strings.HasPrefix(realSrc, "data:image") {
				continue
			}
			alt, _ := img.GetAttribute("alt", playwright.LocatorGetAttributeOptions{
				Timeout: playwright.Float(actionTimeout),
			})
			images = append(images, scraper.ImageItem{URL: realSrc, Alt: alt})
		}
	}

	if realFeaturedSrc != "" {
		featuredAlt, _ := featuredImg.GetAttribute("alt", playwright.LocatorGetAttributeOptions{
			Timeout: playwright.Float(actionTimeout),
		})
		// BUG FIX: src/alt HTML-escape olunmadan atributa yerləşdirilirdi.
		featuredImgTag := fmt.Sprintf(`<img src="%s" alt="%s"><br>`, html.EscapeString(realFeaturedSrc), html.EscapeString(featuredAlt))
		contentHTML = featuredImgTag + contentHTML
	}

	// Excerpt varsa content_html-in əvvəlinə (şəkildən sonra) əlavə et
	// BUG FIX: excerpt InnerText-dən gəlir (plain-text), HTML-ə yerləşdirməzdən
	// əvvəl escape olunmalıdır — əks halda mətndəki "<", ">", "&" simvolları
	// təsadüfən HTML tag/entity kimi parse oluna bilər.
	if excerpt != "" {
		contentHTML = fmt.Sprintf(`<p><strong>%s</strong></p>`, html.EscapeString(excerpt)) + contentHTML
		// Plain text-ə də əlavə et
		content = excerpt + "\n\n" + content
	}

	return scraper.ScrapeResult{
		Item: fi,
		Content: &scraper.ScrapedContent{
			Title:       strings.TrimSpace(title),
			Author:      strings.TrimSpace(author),
			Date:        strings.TrimSpace(date),
			Content:     content,
			ContentHTML: contentHTML,
			Images:      images,
			VideoURL:    "",
		},
	}
}
