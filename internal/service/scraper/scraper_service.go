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
	scrapers     []ScraperEntry
	workerCount  int
	baseURL      string // "/view" linklərini qurmaq üçün, məs. http://localhost:8082
}

func NewScraperService(feedItemRepo domain.FeedItemRepository, scrapers map[string]Scraper, workerCount int, baseURL string) *ScraperService {
	entries := make([]ScraperEntry, 0, len(scrapers))
	for prefix, sc := range scrapers {
		entries = append(entries, ScraperEntry{Prefix: prefix, Scraper: sc})
	}
	return &ScraperService{
		feedItemRepo: feedItemRepo,
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
						); err != nil {
							slog.Error("scraper_service: DB xətası", "item_id", r.Item.ID, "error", err)
							failedCh <- r.Item
							mu.Lock()
							failed++
							mu.Unlock()
							continue
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

	slog.Info("scraper_service: chunk tamamlandı", "success", success, "failed", failed)
	return failedItems
}
