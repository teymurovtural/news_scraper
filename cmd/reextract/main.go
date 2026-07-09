package main

import (
	"context"
	"fmt"
	"log"

	"example.com/new-scraper/internal/config"
	"example.com/new-scraper/internal/platform/database"
	"example.com/new-scraper/internal/repository"
	"example.com/new-scraper/internal/service/scraper"
	"example.com/new-scraper/internal/service/scraper/sources/bleepingcomputer"
	"example.com/new-scraper/internal/service/scraper/sources/cyberscoop"
	"example.com/new-scraper/internal/service/scraper/sources/darkreading"
	"example.com/new-scraper/internal/service/scraper/sources/itsecurityguru"
	"example.com/new-scraper/internal/service/scraper/sources/securityweek"
	"example.com/new-scraper/internal/service/scraper/sources/thehackernews"

	"github.com/playwright-community/playwright-go"
)

// batchSize — hər dəfə DB-dən neçə item çəkiləcək (pagination üçün).
const batchSize = 500

// cmd/reextract — DB-də ARTIQ scrape olunmuş bütün məqalələri YENİ scraper
// kodu ilə yenidən scrape edir.
//
// İSTİFADƏ SSENARİSİ: scraper-lərdə bug fix edəndən sonra (məsələn HTML
// injection, dublikat şəkil, başlıq dublikatı və s. düzəldiləndən sonra),
// köhnə DB sətirlərində hələ də köhnə/xətalı content_html/images qala bilər —
// çünki fix yalnız GƏLƏCƏK scrape-lərə tətbiq olunur, artıq DB-də olan
// sətirlərə avtomatik tətbiq olunmur. Bu alət mövcud sətirləri yeni kodla
// "təzələyir".
//
// DİQQƏT:
//   - Server işləməyəndə çağır (Playwright öz nüsxəsini açır, server də
//     eyni vaxtda işləsə, hər ikisi eyni saytlara paralel müraciət edər).
//   - Bu, DB-dəki BÜTÜN item-ləri yenidən scrape edir — saytlara çoxlu
//     yeni sorğu deməkdir, ehtiyat və rate-limit-lərə diqqətli ol.
//
// İşlətmək: go run ./cmd/reextract
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.NewPostgresDB(cfg.DB.ConnectionString())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	pw, err := playwright.Run()
	if err != nil {
		log.Fatal(fmt.Errorf("playwright başladılmadı: %w", err))
	}
	defer func() {
		if err := pw.Stop(); err != nil {
			log.Printf("playwright dayandırılarkən xəta: %v", err)
		}
	}()

	feedItemRepo := repository.NewFeedItemRepository(db)

	// Qeyd: bu xəritə cmd/server/main.go-dakı ilə eynidir. Əgər gələcəkdə
	// yeni bir mənbə əlavə etsən, ora ilə yanaşı bura da əlavə etməyi unutma.
	scrapers := map[string]scraper.Scraper{
		"https://thehackernews.com":        thehackernews.New(pw, cfg.Playwright.Headless),
		"https://www.darkreading.com":      darkreading.New(pw, cfg.Playwright.Headless),
		"https://www.bleepingcomputer.com": bleepingcomputer.New(pw, cfg.Playwright.Headless),
		"https://cyberscoop.com":           cyberscoop.New(pw, cfg.Playwright.Headless),
		"https://www.itsecurityguru.org":   itsecurityguru.New(pw, cfg.Playwright.Headless),
		"https://www.securityweek.com":     securityweek.New(pw, cfg.Playwright.Headless),
	}

	baseURL := fmt.Sprintf("http://localhost:%s", cfg.Server.Port)
	scraperService := scraper.NewScraperService(feedItemRepo, scrapers, cfg.Poller.WorkerCount, baseURL)

	ctx := context.Background()

	// fetched_at DESC-ə görə pagination edirik (GetAll-un öz ORDER BY-ı).
	// UpdateScrapedData fetched_at-ı dəyişmədiyi üçün, öz yazılarımız
	// pagination-u pozmur (sətirlər "sürüşmür").
	offset := 0
	total := 0
	for {
		items, err := feedItemRepo.GetAll(ctx, batchSize, offset)
		if err != nil {
			log.Fatal(fmt.Errorf("item-lər alınmadı (offset=%d): %w", offset, err))
		}
		if len(items) == 0 {
			break
		}

		log.Printf("Növbə: %d–%d (bu dəfə %d item yenidən scrape ediləcək)", offset, offset+len(items), len(items))
		scraperService.ReextractItems(ctx, items)

		total += len(items)
		offset += batchSize

		if len(items) < batchSize {
			break
		}
	}

	if total == 0 {
		fmt.Println("DB-də heç bir item tapılmadı.")
		return
	}

	fmt.Printf("\nReextract tamamlandı — cəmi %d item yenidən scrape edildi ✅\n", total)
}
