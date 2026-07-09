package base

import (
	"fmt"
	"log/slog"
	"sync"

	"example.com/new-scraper/internal/domain"
	"example.com/new-scraper/internal/service/scraper"

	"github.com/playwright-community/playwright-go"
)

// PageScraper — hər mənbə scraper-i bu interfeysi implement edir.
// Base browser/tab/goroutine işini görür, hazır page-i buraya verir.
type PageScraper interface {
	ScrapePage(page playwright.Page, item domain.FeedItem) scraper.ScrapeResult
}

// RunMultiple — browser açır, hər item üçün ayrı tab-da ScrapePage çağırır.
// Bütün scraper-lər bu funksiyanı istifadə edir.
func RunMultiple(
	pw *playwright.Playwright,
	headless bool,
	items []domain.FeedItem,
	timeoutMs int,
	ps PageScraper,
) []scraper.ScrapeResult {
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	})
	if err != nil {
		results := make([]scraper.ScrapeResult, len(items))
		for i, item := range items {
			results[i] = scraper.ScrapeResult{Item: item, Err: fmt.Errorf("browser açılmadı: %w", err)}
		}
		return results
	}
	defer browser.Close()

	browserCtx, err := browser.NewContext()
	if err != nil {
		results := make([]scraper.ScrapeResult, len(items))
		for i, item := range items {
			results[i] = scraper.ScrapeResult{Item: item, Err: fmt.Errorf("context yaradılmadı: %w", err)}
		}
		return results
	}
	defer browserCtx.Close()

	results := make([]scraper.ScrapeResult, len(items))
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, fi domain.FeedItem) {
			defer wg.Done()

			// PANIC RECOVERY: PageScraper.ScrapePage implementasiyaları
			// (hər mənbənin öz selector məntiqi) gözlənilməz HTML struktur
			// dəyişikliyi ilə qarşılaşanda runtime panic verə bilər (məs.
			// nil slice-dan index oxumaq, gözlənilməz nil pointer və s.).
			// Bu, adi `err` kimi qayıtmır — recover olunmasa, BÜTÜN proses
			// (API server, scheduler, digər bütün worker-lər) dərhal çökür,
			// çünki Go-da bir goroutine-dəki tutulmamış panic bütün prosesi
			// öldürür.
			//
			// Burda recover edərək, panic-i sadəcə BU item üçün normal bir
			// xəta halına çeviririk — item mövcud retry mexanizminə
			// (scraper_service.go-dakı 30s→60s cəhd) düşür, qalan hər şey
			// (digər tab-lar, server) toxunulmadan davam edir.
			defer func() {
				if r := recover(); r != nil {
					slog.Error("base: panic tutuldu", "link", fi.Link, "panic", r)
					results[idx] = scraper.ScrapeResult{Item: fi, Err: fmt.Errorf("panic: %v", r)}
				}
			}()

			page, err := browserCtx.NewPage()
			if err != nil {
				results[idx] = scraper.ScrapeResult{Item: fi, Err: fmt.Errorf("tab açılmadı: %w", err)}
				return
			}

			// WaitUntilStateCommit — server ilk cavab verən kimi davam edir,
			// script/render bitməsini gözləmir. Hər scraper öz WaitForSelector-unu aparır.
			timeout := float64(timeoutMs)
			if _, err := page.Goto(fi.Link, playwright.PageGotoOptions{
				WaitUntil: playwright.WaitUntilStateCommit,
				Timeout:   &timeout,
			}); err != nil {
				page.Close()
				results[idx] = scraper.ScrapeResult{Item: fi, Err: fmt.Errorf("səhifə yüklənmədi: %w", err)}
				return
			}

			results[idx] = ps.ScrapePage(page, fi)
			page.Close()
		}(i, item)
	}

	wg.Wait()
	return results
}
