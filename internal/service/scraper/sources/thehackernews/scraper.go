package thehackernews

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"example.com/new-scraper/internal/domain"
	"example.com/new-scraper/internal/service/scraper"
	"example.com/new-scraper/internal/service/scraper/base"

	"github.com/PuerkitoBio/goquery"
	"github.com/playwright-community/playwright-go"
)

// actionTimeout — InnerText/InnerHTML/GetAttribute/WaitFor kimi bütün DOM
// action-ları üçün vahid timeout. Digər scraper-lərdə (securityweek,
// bleepingcomputer, cyberscoop, itsecurityguru) də eyni pattern istifadə
// olunur — Locator metodları default olaraq 30-60s gözləyir, bu isə hər
// action-u qısa saxlayıb bir linkin bütün poll-u bloklamasının qarşısını alır.
//
// Qeyd: bu fayl `new(actionTimeout)` istifadə edir — Go 1.26-da əlavə olunan
// `new(expr)` sintaksisi (bax go.mod: go 1.26.2), köhnə `playwright.Float(x)`
// köməkçi funksiyasının yerini tutur, nəticə eynidir (*float64 pointer).
const actionTimeout = float64(6000)

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

// ScrapePage — yalnız The Hacker News-a xas selector məntiqidir.
func (s *Scraper) ScrapePage(page playwright.Page, fi domain.FeedItem) scraper.ScrapeResult {
	// BUG FIX: page.Goto WaitUntilStateCommit ilə çağırılır (base.go) — yəni
	// server ilk cavab verən kimi davam edir, DOM hələ parse olunmamış ola
	// bilər. Aşağıdakı raw page.Evaluate çağırışı isə Locator metodlarının
	// əksinə auto-wait ETMİR, ona görə "document.body" hələ null olanda
	// "Cannot read properties of null (reading 'scrollHeight')" xətası verir.
	// Scroll etməzdən əvvəl body-nin DOM-a "attach" olduğuna əmin oluruq.
	if err := page.Locator("body").WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateAttached,
		Timeout: new(actionTimeout),
	}); err != nil {
		slog.Warn("thehackernews: body gözlənilmədi", "link", fi.Link, "error", err)
	}

	if _, err := page.Evaluate(`() => window.scrollTo(0, document.body.scrollHeight)`); err != nil {
		slog.Warn("thehackernews: scroll xətası", "link", fi.Link, "error", err)
	}
	time.Sleep(1 * time.Second)
	if _, err := page.Evaluate(`() => window.scrollTo(0, 0)`); err != nil {
		slog.Warn("thehackernews: scroll geri xətası", "link", fi.Link, "error", err)
	}
	time.Sleep(500 * time.Millisecond)

	title, err := page.Locator("h1.story-title a").InnerText(playwright.LocatorInnerTextOptions{
		Timeout: new(actionTimeout),
	})
	if err != nil {
		slog.Warn("thehackernews: title alınmadı", "link", fi.Link, "error", err)
	}

	postmeta := page.Locator("div.postmeta")
	author, err := postmeta.Locator("span.author").First().InnerText(playwright.LocatorInnerTextOptions{
		Timeout: new(actionTimeout),
	})
	if err != nil {
		slog.Warn("thehackernews: author alınmadı", "link", fi.Link, "error", err)
	}

	date, err := postmeta.Locator("span.author").Nth(1).InnerText(playwright.LocatorInnerTextOptions{
		Timeout: new(actionTimeout),
	})
	if err != nil {
		slog.Warn("thehackernews: tarix alınmadı", "link", fi.Link, "error", err)
	}

	// #articlebody-nin xam HTML-i — struktur (paraqraf sırası, şəkil yerləri) qorunur
	rawContentHTML, err := page.Locator("div#articlebody").InnerHTML(playwright.LocatorInnerHTMLOptions{
		Timeout: new(actionTimeout),
	})
	if err != nil {
		slog.Warn("thehackernews: content HTML alınmadı", "link", fi.Link, "error", err)
	}

	// Lazy load şəkillərdə src placeholder-dır, əsl URL data-src-dadır.
	// goquery ilə data-src olan hər img-in src-ini əvəz edirik.
	rawContentHTML = resolveLazyImages(rawContentHTML)

	// Bu saytda "articlebody" daxilində olan, məqaləyə aid olmayan bloklar:
	//   - div#hiddenH1     → boş JS marker
	//   - div.cf.note-b    → "Follow us on Google News/Twitter/LinkedIn" təşviqatı
	//   - div.dog_two      → sponsorlu reklam bloku (rel="sponsored" ilə təsdiqlənir)
	//   - div.recap-ad     → "Weekly Recap" məqalələrində sponsorlu reklam bloku
	// Qeyd: div.td-wrap ThreatsDay digest siyahısını saxlayır — silinmir.
	removeSelectors := []string{
		"div#hiddenH1",
		"div.cf.note-b",
		"div.dog_two",
		"div.recap-ad",
	}

	contentHTML, err := base.CleanArticleHTML(rawContentHTML, removeSelectors)
	if err != nil {
		slog.Warn("thehackernews: content HTML təmizlənmədi", "link", fi.Link, "error", err)
		contentHTML = rawContentHTML // təmizləmə uğursuz olsa, xam versiyanı itirmə
	}

	content, err := base.HTMLToPlainText(contentHTML)
	if err != nil {
		slog.Warn("thehackernews: plain text çıxarılmadı", "link", fi.Link, "error", err)
	}

	// Featured image div.separator içindədir və articlebody-nin bir hissəsidir —
	// contentHTML-ə artıq daxildir. Yalnız images array-ı üçün ayrıca götürürük.
	var featuredSrc, featuredAlt string
	featuredImgLoc := page.Locator("div.separator img").First()
	if src, err := featuredImgLoc.GetAttribute("src", playwright.LocatorGetAttributeOptions{
		Timeout: new(actionTimeout),
	}); err == nil && src != "" && !strings.HasPrefix(src, "data:image") {
		featuredSrc = src
		featuredAlt, _ = featuredImgLoc.GetAttribute("alt", playwright.LocatorGetAttributeOptions{
			Timeout: new(actionTimeout),
		})
	}

	var images []scraper.ImageItem
	imgLocators, err := page.Locator("div#articlebody img").All()
	if err != nil {
		slog.Warn("thehackernews: şəkillər alınmadı", "link", fi.Link, "error", err)
	} else {
		for _, img := range imgLocators {
			// Ortaq lazy-load məntiqi (data-src → src → data: URI rədd) base
			// paketindədir — əvvəllər bu blok bir neçə scraper-də təkrarlanırdı.
			realURL, alt := base.ExtractLazyImageAttr(img, actionTimeout)
			if realURL == "" {
				continue
			}
			if strings.Contains(strings.ToLower(realURL), "-d.png") ||
				strings.Contains(strings.ToLower(realURL), "sponsor") {
				continue
			}

			images = append(images, scraper.ImageItem{URL: realURL, Alt: alt})
		}
	}

	videoURL := ""
	iframeLocators, err := page.Locator("div#articlebody iframe[src*='youtube.com/embed']").All()
	if err != nil {
		slog.Warn("thehackernews: iframe alınmadı", "link", fi.Link, "error", err)
	} else if len(iframeLocators) > 0 {
		src, _ := iframeLocators[0].GetAttribute("src", playwright.LocatorGetAttributeOptions{Timeout: new(actionTimeout)})
		if src != "" {
			videoURL = src
			slog.Info("thehackernews: video tapıldı", "link", fi.Link, "video_url", videoURL)
		}
	}

	// BUG FIX: featured image div.separator-un içindədir, div.separator isə
	// div#articlebody-nin bir hissəsidir — ona görə yuxarıdakı ümumi loop
	// (div#articlebody img) artıq bu şəkli tutur. Əvvəlki kod bunu yoxlamadan
	// YENƏ əvvələ əlavə edirdi — nəticədə eyni şəkil images array-ında 2 dəfə
	// olurdu. İndi əvvəlcə yoxlayırıq ki, artıq mövcuddurmu.
	if featuredSrc != "" {
		alreadyPresent := false
		for _, img := range images {
			if img.URL == featuredSrc {
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			images = append([]scraper.ImageItem{{URL: featuredSrc, Alt: featuredAlt}}, images...)
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
// The Hacker News ll-lazy sistemi istifadə edir: src = 1px placeholder, əsl URL = data-src.
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
