package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"example.com/new-scraper/internal/domain"
)

const tabsPerWorker = 5

// ScraperEntry — prefix ilə scraper-i birlikdə saxlayır.
// Slice istifadə edilir ki, iteration sırası deterministik olsun.
type ScraperEntry struct {
	Prefix  string
	Scraper Scraper
}

type ScraperService struct {
	feedItemRepo domain.FeedItemRepository
	sourceRepo   domain.SourceRepository // sağlamlıq siqnalları (fail/reset) üçün — bax updateSourceHealth
	scrapers     []ScraperEntry
	workerCount  int
	baseURL      string // "/view" linklərini qurmaq üçün, məs. http://localhost:8082
}

func NewScraperService(feedItemRepo domain.FeedItemRepository, sourceRepo domain.SourceRepository, scrapers map[string]Scraper, workerCount int, baseURL string) *ScraperService {
	entries := make([]ScraperEntry, 0, len(scrapers))
	for prefix, sc := range scrapers {
		entries = append(entries, ScraperEntry{Prefix: prefix, Scraper: sc})
	}
	return &ScraperService{
		feedItemRepo: feedItemRepo,
		sourceRepo:   sourceRepo,
		scrapers:     entries,
		workerCount:  workerCount,
		baseURL:      baseURL,
	}
}

// ReextractItems — verilmiş item-ləri (onsuz da scrape olunmuş olsalar belə)
// YENİDƏN scrape edir. `cmd/reextract` aləti tərəfindən istifadə olunur —
// scraper kodunda bug fix ediləndən sonra, köhnə (artıq scrape olunmuş)
// DB sətirlərini yeni məntiqlə yenidən "təzələmək" üçün. `ScrapeUnscraped`-in
// eyni 30s→60s retry pattern-ini istifadə edir.
func (s *ScraperService) ReextractItems(ctx context.Context, items []domain.FeedItem) {
	if len(items) == 0 {
		return
	}

	failed := s.scrapeItems(ctx, items, 30000)

	if len(failed) > 0 {
		slog.Info("scraper_service: yenidən cəhd (60s)", "count", len(failed))
		time.Sleep(2 * time.Second)
		s.scrapeItems(ctx, failed, 60000)
	}
}

func (s *ScraperService) ScrapeUnscraped(ctx context.Context) {
	items, err := s.feedItemRepo.GetUnscraped(ctx, 500)
	if err != nil {
		slog.Error("scraper_service: unscraped linklər alınmadı", "error", err)
		return
	}

	if len(items) == 0 {
		slog.Info("scraper_service: scrape ediləcək yeni link yoxdur")
	} else {
		slog.Info("scraper_service: scrape başlayır", "count", len(items))

		failed := s.scrapeItems(ctx, items, 30000)

		if len(failed) > 0 {
			slog.Info("scraper_service: yenidən cəhd (60s)", "count", len(failed))
			time.Sleep(2 * time.Second)
			s.scrapeItems(ctx, failed, 60000)
		}
	}

	// Əvvəlki polllardan boş content qalan məqalələri retry et
	if ctx.Err() == nil {
		s.retryEmptyContent(ctx)
	}
}

func (s *ScraperService) retryEmptyContent(ctx context.Context) {
	items, err := s.feedItemRepo.GetEmptyContent(ctx, 50)
	if err != nil {
		slog.Error("scraper_service: boş content sorğusu xətası", "error", err)
		return
	}

	if len(items) == 0 {
		return
	}

	slog.Info("scraper_service: boş content-li məqalələr yenidən çəkilir", "count", len(items))
	time.Sleep(3 * time.Second)
	s.scrapeItems(ctx, items, 60000)
}

func (s *ScraperService) getScraperForLink(link string) Scraper {
	for _, entry := range s.scrapers {
		if len(link) >= len(entry.Prefix) && link[:len(entry.Prefix)] == entry.Prefix {
			return entry.Scraper
		}
	}
	return nil
}

func chunkItems(items []domain.FeedItem, size int) [][]domain.FeedItem {
	var chunks [][]domain.FeedItem
	for len(items) >= size {
		chunks = append(chunks, items[:size])
		items = items[size:]
	}
	if len(items) > 0 {
		chunks = append(chunks, items)
	}
	return chunks
}

// groupAndChunk — item-ləri ƏVVƏLCƏ mənbəyə (scraper-ə) görə qruplaşdırır,
// SONRA hər qrupu öz daxilində `size`-lik chunk-lara bölür.
//
// BUG FIX: köhnə versiyada bütün item-lər (fərqli mənbələrdən qarışıq)
// birbaşa 5-lik chunk-lara bölünürdü, sonra chunk[0]-ın linkinə görə TEK BİR
// scraper seçilib bütün chunk ona verilirdi. Nəticədə bir chunk-da fərqli
// mənbələrdən link olsaydı, onlardan bəziləri YANLIŞ scraper (yanlış CSS
// selector-larla) ilə açılırdı — bu da səssizcə boş/yanlış content yaradırdı.
//
// İndi hər chunk qabaqcadan tək mənbədən təmin olunur, ona görə bu problem
// mümkün deyil.
//
// Əlavə olaraq, çıxan chunk-lar mənbələr arasında round-robin sırayla
// qatarlanır (1-ci chunk hər mənbədən, sonra 2-ci chunk hər mənbədən...).
// Bu, worker-lərin eyni anda tək bir saytı "yağdırmasının" qarşısını alır —
// əks halda məsələn 5 worker də ardıcıl olaraq eyni saytın chunk-larını
// götürüb sayta paralel zərbə vura bilər (rate-limit/Cloudflare riski artar).
func (s *ScraperService) groupAndChunk(items []domain.FeedItem, size int) (chunks [][]domain.FeedItem, unmatched []domain.FeedItem) {
	groups := make(map[Scraper][]domain.FeedItem)
	var order []Scraper // ilk rastlaşdığımız sıra ilə saxlanır ki, nəticə deterministik olsun

	for _, item := range items {
		sc := s.getScraperForLink(item.Link)
		if sc == nil {
			unmatched = append(unmatched, item)
			continue
		}
		if _, exists := groups[sc]; !exists {
			order = append(order, sc)
		}
		groups[sc] = append(groups[sc], item)
	}

	var perGroupChunks [][][]domain.FeedItem
	maxLen := 0
	for _, sc := range order {
		gc := chunkItems(groups[sc], size)
		perGroupChunks = append(perGroupChunks, gc)
		if len(gc) > maxLen {
			maxLen = len(gc)
		}
	}

	for i := 0; i < maxLen; i++ {
		for _, gc := range perGroupChunks {
			if i < len(gc) {
				chunks = append(chunks, gc[i])
			}
		}
	}

	return chunks, unmatched
}

func (s *ScraperService) scrapeItems(ctx context.Context, items []domain.FeedItem, timeoutMs int) []domain.FeedItem {
	chunks, unmatched := s.groupAndChunk(items, tabsPerWorker)

	jobs := make(chan []domain.FeedItem, len(chunks))
	failedCh := make(chan domain.FeedItem, len(items))

	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	failed := 0

	// Heç bir scraper-ə uyğun gəlməyən linklər (yeni/naməlum domain və s.) —
	// browser açmadan birbaşa uğursuz sayılır.
	for _, item := range unmatched {
		slog.Warn("scraper_service: scraper tapılmadı", "link", item.Link)
		failedCh <- item
		failed++
	}

	for w := 0; w < s.workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for chunk := range jobs {
				if len(chunk) == 0 {
					continue
				}

				// PANIC RECOVERY: bu blok həm sc.ScrapeMultiple çağırışını,
				// həm də nəticələrin emalını (DB yazısı, image mapping)
				// əhatə edir. Panic olsa (gözlənilməz nil/format xətası və
				// s.), yalnız BU chunk-dakı item-lər uğursuz sayılıb retry
				// növbəsinə düşür — worker özü, digər worker-lər və bütün
				// proses ayaqda qalır. Panic recover olunmasaydı, Go-da bir
				// goroutine-dəki tutulmamış panic bütün prosesi (API server
				// daxil) dərhal öldürərdi.
				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("scraper_service: panic tutuldu", "chunk_size", len(chunk), "panic", r)
							for _, item := range chunk {
								failedCh <- item
							}
							mu.Lock()
							failed += len(chunk)
							mu.Unlock()
						}
					}()

					sc := s.getScraperForLink(chunk[0].Link)
					if sc == nil {
						slog.Warn("scraper_service: scraper tapılmadı", "link", chunk[0].Link)
						for _, item := range chunk {
							failedCh <- item
						}
						mu.Lock()
						failed += len(chunk)
						mu.Unlock()
						return
					}

					results := sc.ScrapeMultiple(ctx, chunk, timeoutMs)

					for _, r := range results {
						if r.Err != nil {
							slog.Error("scraper_service: scrape uğursuz", "item_id", r.Item.ID, "error", r.Err)
							failedCh <- r.Item
							mu.Lock()
							failed++
							mu.Unlock()
							continue
						}

						images := make([]domain.ImageItem, 0, len(r.Content.Images))
						for _, img := range r.Content.Images {
							images = append(images, domain.ImageItem{
								URL: img.URL,
								Alt: img.Alt,
							})
						}

						viewURL := ""
						if s.baseURL != "" {
							viewURL = fmt.Sprintf("%s/api/v1/items/%d/view", s.baseURL, r.Item.ID)
						}

						// CVE ID-lərini title+content-dən çıxarırıq (bax cve.go)
						// — eyni CVE-ni paylaşan məqalələri sonradan
						// əlaqələndirmək üçün (dedup/related-coverage) əsas.
						cveIDs := ExtractCVEIDs(r.Content.Title + " " + r.Content.Content)

						if err := s.feedItemRepo.UpdateScrapedData(
							ctx,
							r.Item.ID,
							r.Content.Title,
							r.Content.Author,
							r.Content.Date,
							r.Content.Content,
							r.Content.ContentHTML,
							viewURL,
							images,
							r.Content.VideoURL,
							cveIDs,
						); err != nil {
							slog.Error("scraper_service: DB xətası", "item_id", r.Item.ID, "error", err)
							failedCh <- r.Item
							mu.Lock()
							failed++
							mu.Unlock()
							continue
						}

						// GERİYƏ-DÖNÜK BAYRAQ YENİLƏMƏSİ: bu item CVE ilə
						// yazılıbsa, onu paylaşan BÜTÜN item-lərin (köhnə +
						// bu yeni) has_related_cve sahəsi yenidən hesablanır
						// (bax domain/repositories.go-dakı UpdateRelatedCVEFlags
						// şərhi — niyə bu addım vacibdir). Best-effort: xəta
						// olsa belə əsas item artıq uğurla yazılıb, ona görə
						// item-i uğursuz SAYMIRIQ, sadəcə loglayırıq.
						if len(cveIDs) > 0 {
							if err := s.feedItemRepo.UpdateRelatedCVEFlags(ctx, cveIDs); err != nil {
								slog.Error("scraper_service: has_related_cve yenilənmədi", "item_id", r.Item.ID, "error", err)
							}
						}

						slog.Info("scraper_service: scrape uğurlu", "item_id", r.Item.ID, "title", r.Content.Title)
						mu.Lock()
						success++
						mu.Unlock()
					}
				}()
			}
		}()
	}

	for _, chunk := range chunks {
		jobs <- chunk
	}
	close(jobs)

	wg.Wait()
	close(failedCh)

	var failedItems []domain.FeedItem
	for item := range failedCh {
		failedItems = append(failedItems, item)
	}

	s.updateSourceHealth(ctx, items, failedItems)

	slog.Info("scraper_service: chunk tamamlandı", "success", success, "failed", failed)
	return failedItems
}

// updateSourceHealth — bu scrapeItems çağırışında iştirak edən hər mənbə
// üçün fail_count-u yeniləyir. Məntiq: mənbədən ən azı BİR item uğurla
// scrape olunubsa (yəni content faktiki axır), sağlam sayılır və sıfırlanır.
// Yalnız cəhd edilən bütün item-lər uğursuz olubsa (tam sükut), fail_count
// artırılır.
//
// DİZAYN QEYDİ: bu, item-səviyyəli deyil, mənbə-səviyyəli (aggregate) qərardır
// — bir mənbədən 9 item uğurlu, 1-i uğursuz olsa, bu "əsasən sağlamdır" sayılır
// və reset edilir (yalnız o 1 item retry növbəsinə düşür, mənbənin ümumi
// sağlamlığına təsir etmir). Bu, RSS-in yaxşı işlədiyi, amma scrape-in HƏR
// item üçün uğursuz olduğu halı (məs. sayt HTML strukturunu dəyişib) tutmaq
// üçündür — əvvəllər belə hallar heç cür aşkarlanmırdı (bax
// source_repository.go-dakı UpdateLastPolled şərhi).
func (s *ScraperService) updateSourceHealth(ctx context.Context, attempted, failed []domain.FeedItem) {
	if s.sourceRepo == nil {
		return // testlərdə/nadir hallarda sourceRepo verilməyə bilər
	}

	failedIDs := make(map[int64]bool, len(failed))
	for _, item := range failed {
		failedIDs[item.ID] = true
	}

	succeeded := make(map[int64]bool)
	allFailed := make(map[int64]bool)
	for _, item := range attempted {
		if failedIDs[item.ID] {
			allFailed[item.SourceID] = true
		} else {
			succeeded[item.SourceID] = true
		}
	}

	for sourceID := range succeeded {
		if err := s.sourceRepo.ResetFailCount(ctx, sourceID); err != nil {
			slog.Error("scraper_service: fail_count sıfırlanmadı", "source_id", sourceID, "error", err)
		}
	}

	for sourceID := range allFailed {
		if succeeded[sourceID] {
			continue // bu mənbədən ən azı 1 uğur var idi, tam uğursuz sayılmır
		}
		deactivated, err := s.sourceRepo.IncrementFailCount(ctx, sourceID)
		if err != nil {
			slog.Error("scraper_service: fail_count artırılmadı", "source_id", sourceID, "error", err)
			continue
		}
		if deactivated {
			slog.Warn("scraper_service: XƏBƏRDARLIQ — mənbə ardıcıl scrape uğursuzluqlarına görə avtomatik deaktiv edildi", "source_id", sourceID)
		}
	}
}
