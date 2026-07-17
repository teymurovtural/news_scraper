package generic

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"

	"example.com/new-scraper/internal/domain"
	"example.com/new-scraper/internal/service/scraper"
	"example.com/new-scraper/internal/service/scraper/base"

	"github.com/PuerkitoBio/goquery"
	"github.com/playwright-community/playwright-go"
)

const defaultActionTimeoutMs = float64(5000)

// Scraper — SourceConfig-ə əsasən istənilən saytı scrape edə bilən VAHİD
// implementasiya. 6 mövcud sayta-xas scraper paketinin (thehackernews,
// securityweek, bleepingcomputer, cyberscoop, itsecurityguru, darkreading)
// yerini tutur — həmin paketlər hələ də repoda saxlanılıb (referans/fallback
// üçün), amma cmd/server/main.go artıq onları import etmir.
type Scraper struct {
	pw       *playwright.Playwright
	headless bool
	cfg      SourceConfig
}

func New(pw *playwright.Playwright, headless bool, cfg SourceConfig) *Scraper {
	return &Scraper{pw: pw, headless: headless, cfg: cfg}
}

// BuildScrapers — hər SourceConfig üçün bir Scraper yaradıb, ScraperService-in
// gözlədiyi prefix→Scraper map-ini qaytarır.
func BuildScrapers(pw *playwright.Playwright, headless bool, configs []SourceConfig) map[string]scraper.Scraper {
	out := make(map[string]scraper.Scraper, len(configs))
	for _, cfg := range configs {
		out[cfg.Prefix] = New(pw, headless, cfg)
	}
	return out
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

// ScrapePage — base.PageScraper interfeysini implement edir. Bütün sayta-xas
// davranış cfg (SourceConfig) daxilindədir, bu funksiyanın özü heç bir sayta
// bağlı deyil.
func (s *Scraper) ScrapePage(page playwright.Page, fi domain.FeedItem) scraper.ScrapeResult {
	cfg := s.cfg
	timeout := cfg.ActionTimeoutMs
	if timeout <= 0 {
		timeout = defaultActionTimeoutMs
	}

	for _, h := range cfg.PreHooks {
		runHook(page, h, timeout, cfg.Name, fi.Link)
	}

	title := s.extractField(page, cfg.Title, timeout, fi.Link)
	author := s.extractField(page, cfg.Author, timeout, fi.Link)
	date := s.extractField(page, cfg.Date, timeout, fi.Link)

	excerpt := s.extractExcerpt(page, cfg.Excerpt, timeout, fi.Link)

	for _, h := range cfg.MidHooks {
		runHook(page, h, timeout, cfg.Name, fi.Link)
	}

	rawContentHTML, err := page.Locator(cfg.ContentSelector).First().InnerHTML(playwright.LocatorInnerHTMLOptions{
		Timeout: playwright.Float(timeout),
	})
	if err != nil {
		slog.Warn("generic: content HTML alınmadı", "source", cfg.Name, "link", fi.Link, "error", err)
	}

	if cfg.ResolveLazyImagesInContentHTML {
		rawContentHTML = resolveLazyImagesInHTML(rawContentHTML)
	}

	// excerpt.IsHTML=true (cyberscoop pattern) — excerpt CLEANING-dən ƏVVƏL
	// content-ə qatılır, yəni remove_selectors ona da tətbiq olunur.
	combined := rawContentHTML
	if excerpt != nil && excerpt.IsHTML {
		combined = excerpt.HTML + "\n" + combined
	}

	contentHTML, err := base.CleanArticleHTML(combined, cfg.RemoveSelectors)
	if err != nil {
		slog.Warn("generic: content HTML təmizlənmədi", "source", cfg.Name, "link", fi.Link, "error", err)
		contentHTML = combined
	}

	content, err := base.HTMLToPlainText(contentHTML)
	if err != nil {
		slog.Warn("generic: plain text çıxarılmadı", "source", cfg.Name, "link", fi.Link, "error", err)
	}

	images := s.extractImages(page, cfg, timeout, fi.Link, &contentHTML)
	videoURL := s.extractVideo(page, cfg, timeout, fi.Link)

	// excerpt.IsHTML=false (securityweek pattern) — CLEANING-dən VƏ featured
	// image prepend-indən SONRA, ən başa əlavə olunur (remove_selectors-a
	// tabe deyil, çünki artıq öz plain-text mənbəyindən gəlir).
	if excerpt != nil && !excerpt.IsHTML {
		contentHTML = excerpt.HTML + contentHTML
		content = excerpt.Plain + "\n\n" + content
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

// extractField — FieldSelector-a əsasən bir mətn sahəsini (title/author/date)
// çıxarır: scope+nth ilə DOM-dan tapır, meta_first sırasına uyğun meta
// fallback tətbiq edir, sonda trim/title-case emalı aparır.
func (s *Scraper) extractField(page playwright.Page, fs FieldSelector, timeout float64, link string) string {
	tryDOM := func() string {
		if fs.Selector == "" {
			return ""
		}
		var loc playwright.Locator
		if fs.Scope != "" {
			loc = page.Locator(fs.Scope).Locator(fs.Selector)
		} else {
			loc = page.Locator(fs.Selector)
		}
		val, err := loc.Nth(fs.Nth).InnerText(playwright.LocatorInnerTextOptions{
			Timeout: playwright.Float(timeout),
		})
		if err != nil {
			slog.Warn("generic: sahə DOM-dan alınmadı", "source", s.cfg.Name, "link", link, "selector", fs.Selector, "error", err)
		}
		return val
	}
	tryMeta := func() string {
		if fs.MetaFallback == "" {
			return ""
		}
		val, err := page.Locator(fs.MetaFallback).GetAttribute("content", playwright.LocatorGetAttributeOptions{
			Timeout: playwright.Float(timeout),
		})
		if err != nil {
			return ""
		}
		return val
	}

	var val string
	if fs.MetaFirst {
		val = tryMeta()
		if strings.TrimSpace(val) == "" {
			val = tryDOM()
		}
	} else {
		val = tryDOM()
		if strings.TrimSpace(val) == "" {
			val = tryMeta()
		}
	}

	val = strings.TrimSpace(val)
	if fs.TrimLeftChars != "" {
		val = strings.TrimSpace(strings.TrimLeft(val, fs.TrimLeftChars))
	}
	if fs.TrimRightChars != "" {
		val = strings.TrimRight(val, fs.TrimRightChars)
	}
	if fs.TitleCase {
		val = toTitleCase(val)
	}
	return val
}

// excerptResult — bir excerpt çıxarımının nəticəsi. Plain yalnız IsHTML=false
// olduqda dolur (plain-text content sahəsinə əlavə olunmaq üçün lazımdır).
type excerptResult struct {
	HTML   string
	Plain  string
	IsHTML bool
}

// extractExcerpt — varsa, excerpt-i "<p><strong>...</strong></p>" HTML
// fraqmenti kimi qaytarır. Tapılmasa/boşdursa nil qaytarır.
func (s *Scraper) extractExcerpt(page playwright.Page, ex *ExcerptConfig, timeout float64, link string) *excerptResult {
	if ex == nil {
		return nil
	}

	if ex.IsHTML {
		raw, err := page.Locator(ex.Selector).First().InnerHTML(playwright.LocatorInnerHTMLOptions{
			Timeout: playwright.Float(timeout),
		})
		raw = strings.TrimSpace(raw)
		if err != nil || raw == "" {
			return nil
		}
		return &excerptResult{HTML: "<p><strong>" + raw + "</strong></p>", IsHTML: true}
	}

	raw, err := page.Locator(ex.Selector).First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(timeout),
	})
	raw = strings.TrimSpace(raw)
	if err != nil || raw == "" {
		return nil
	}
	// BUG FIX pattern (securityweek-dən götürülüb): InnerText plain mətndir,
	// HTML-ə yerləşdirməzdən əvvəl escape olunmalıdır — əks halda mətndəki
	// "<", ">", "&" simvolları təsadüfən tag/entity kimi parse oluna bilər.
	return &excerptResult{
		HTML:   fmt.Sprintf("<p><strong>%s</strong></p>", html.EscapeString(raw)),
		Plain:  raw,
		IsHTML: false,
	}
}

// extractImages — featured + content şəkillərini birləşdirir, dublikatları
// (URL üzərindən) süzür. FeaturedImage.Prepend=true olsa, contentHTML-in
// əvvəlinə əl ilə <img> tag-ı əlavə edilir (çünki featured image fiziki
// olaraq ContentSelector-un xaricindədir).
func (s *Scraper) extractImages(page playwright.Page, cfg SourceConfig, timeout float64, link string, contentHTML *string) []scraper.ImageItem {
	var images []scraper.ImageItem

	if cfg.FeaturedImage != nil {
		floc := page.Locator(cfg.FeaturedImage.Selector).First()
		src, alt := base.ExtractLazyImageAttr(floc, timeout)
		if src != "" {
			if cfg.FeaturedImage.ResolveURL {
				src = base.ResolveURL(page.URL(), src)
			}
			images = append(images, scraper.ImageItem{URL: src, Alt: alt})
			if cfg.FeaturedImage.Prepend {
				*contentHTML = fmt.Sprintf(`<img src="%s" alt="%s"><br>`, html.EscapeString(src), html.EscapeString(alt)) + *contentHTML
			}
		}
	}

	if cfg.ContentImages != nil {
		locs, err := page.Locator(cfg.ContentImages.Selector).All()
		if err != nil {
			slog.Warn("generic: şəkillər alınmadı", "source", cfg.Name, "link", link, "error", err)
		} else {
			for _, img := range locs {
				src, alt := base.ExtractLazyImageAttr(img, timeout)
				if src == "" {
					continue
				}

				lowerSrc := strings.ToLower(src)
				if len(cfg.ContentImages.ExcludeContains) > 0 && containsAny(lowerSrc, cfg.ContentImages.ExcludeContains) {
					continue
				}
				if len(cfg.ContentImages.DomainContains) > 0 && !containsAny(src, cfg.ContentImages.DomainContains) {
					continue
				}
				if len(cfg.ContentImages.PathContains) > 0 && !containsAny(src, cfg.ContentImages.PathContains) {
					continue
				}

				dup := false
				for _, existing := range images {
					if existing.URL == src {
						dup = true
						break
					}
				}
				if dup {
					continue
				}

				images = append(images, scraper.ImageItem{URL: src, Alt: alt})
			}
		}
	}

	return images
}

// extractVideo — YouTube iframe embed-i ("iframe_youtube") ya da link-based
// video ("anchor_href") axtarır. Video tapılmasa boş sətir qaytarır.
func (s *Scraper) extractVideo(page playwright.Page, cfg SourceConfig, timeout float64, link string) string {
	if cfg.Video == nil {
		return ""
	}

	var videoURL string

	switch cfg.Video.Mode {
	case "iframe_youtube":
		scopeSel := cfg.Video.Scope
		if scopeSel == "" {
			scopeSel = cfg.ContentSelector
		}
		iframes, err := page.Locator(scopeSel).Locator("iframe[src*='youtube.com/embed']").All()
		if err != nil {
			slog.Warn("generic: iframe alınmadı", "source", cfg.Name, "link", link, "error", err)
		} else if len(iframes) > 0 {
			src, _ := iframes[0].GetAttribute("src", playwright.LocatorGetAttributeOptions{Timeout: playwright.Float(timeout)})
			videoURL = src
		}

	case "anchor_href":
		href, err := page.Locator(cfg.Video.Selector).First().GetAttribute("href", playwright.LocatorGetAttributeOptions{
			Timeout: playwright.Float(timeout),
		})
		if err == nil {
			videoURL = href
		}
	}

	if videoURL != "" {
		slog.Info("generic: video tapıldı", "source", cfg.Name, "link", link, "video_url", videoURL)
	}
	return videoURL
}

// resolveLazyImagesInHTML — data-src olan img teqlərinin src-ini data-src
// dəyəri ilə əvəz edir. Bir çox sayt lazy-load üçün bunu istifadə edir: src
// = placeholder, əsl URL = data-src. (thehackernews/bleepingcomputer-dəki
// eyni adlı funksiyaların ümumiləşdirilmiş versiyası.)
func resolveLazyImagesInHTML(rawHTML string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return rawHTML
	}

	doc.Find("img[data-src]").Each(func(_ int, sel *goquery.Selection) {
		if dataSrc, exists := sel.Attr("data-src"); exists && dataSrc != "" {
			sel.SetAttr("src", dataSrc)
			sel.RemoveAttr("data-src")
		}
	})

	result, err := doc.Find("body").First().Html()
	if err != nil {
		return rawHTML
	}
	return strings.TrimSpace(result)
}

// toTitleCase — "TIM STARKS" → "Tim Starks" (cyberscoop-un CSS
// text-transform:uppercase render effektini normalize etmək üçün).
func toTitleCase(str string) string {
	words := strings.Fields(strings.ToLower(str))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// containsAny — s-in candidates siyahısındakı istənilən substring-i
// daşıyıb-daşımadığını yoxlayır (OR məntiqi).
func containsAny(s string, candidates []string) bool {
	for _, c := range candidates {
		if strings.Contains(s, c) {
			return true
		}
	}
	return false
}
