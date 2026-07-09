package scheduler

import (
	"context"
	"log/slog"
	"time"

	"example.com/new-scraper/internal/service/exporter"
	"example.com/new-scraper/internal/service/fetcher"
	"example.com/new-scraper/internal/service/scraper"
)

type Scheduler struct {
	fetcherService  *fetcher.FetcherService
	scraperService  *scraper.ScraperService
	exporterService *exporter.ExporterService
	interval        time.Duration
	workerCount     int
	stopCh          chan struct{}
	doneCh          chan struct{}
}

func NewScheduler(
	fetcherService *fetcher.FetcherService,
	scraperService *scraper.ScraperService,
	exporterService *exporter.ExporterService,
	intervalSeconds int,
	workerCount int,
) *Scheduler {
	return &Scheduler{
		fetcherService:  fetcherService,
		scraperService:  scraperService,
		exporterService: exporterService,
		interval:        time.Duration(intervalSeconds) * time.Second,
		workerCount:     workerCount,
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	slog.Info("scheduler başladı", "interval", s.interval.String(), "worker_count", s.workerCount)

	go func() {
		// BUG FIX (graceful shutdown): əvvəllər Stop() sadəcə stopCh-i
		// bağlayırdı və çağıran tərəf bunun faktiki nə vaxt təsir etdiyini
		// bilmirdi — main.go dərhal server.Shutdown/db.Close-a keçirdi, halbuki
		// bu goroutine hələ də (uzun sürən) bir poll dövrəsinin ortasında ola
		// bilərdi. İndi goroutine həqiqətən çıxanda (hər hansı səbəbdən:
		// stopCh, ctx.Done) doneCh bağlanır — Wait(timeout) bunu gözləyə bilir.
		defer close(s.doneCh)

		s.run(ctx)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.run(ctx)
			case <-s.stopCh:
				slog.Info("scheduler dayandı")
				return
			case <-ctx.Done():
				slog.Info("scheduler: context ləğv edildi")
				return
			}
		}
	}()
}

// Stop — YENİ poll dövrünün başlamasının qarşısını alır (stopCh bağlanır).
// DİQQƏT: bu, davam edən (in-flight) bir poll dövrəsini DƏRHAL kəsmir —
// stopCh yalnız `run()` bitib ticker-in select-inə qayıdanda yoxlanılır.
// Davam edən dövrənin faktiki bitməsini gözləmək üçün Wait(timeout) istifadə et.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

// Wait — daxili goroutine-in HƏQİQƏTƏN dayanmasını (doneCh bağlanmasını),
// verilmiş timeout-a qədər gözləyir.
//
// true qaytarırsa: scheduler tam təmiz dayanıb, DB/Playwright indi
// bağlanmaq üçün TƏHLÜKƏSİZDİR.
//
// false qaytarırsa: timeout bitib, amma goroutine hələ davam edir — çox
// güman, uzun sürən bir scrape mərhələsi (Playwright, şəbəkə) hələ
// bitməyib. TAM (Go context-səviyyəsində) ləğv etmək üçün scraper/base
// qatına context ötürülməli və Playwright çağırışları ona bağlanmalıdır —
// bu, hazırkı fix-in əhatəsindən kənar, ayrıca bir refaktorinq işidir.
// Qısamüddətli təhlükəsiz həll: main.go bu halda xəbərdarlıq loglayıb yenə
// də shutdown-a davam edir (server prosesi özü bağlanır, arxa planda qalan
// goroutine OS tərəfindən proses bitəndə ləğv olunur).
func (s *Scheduler) Wait(timeout time.Duration) bool {
	select {
	case <-s.doneCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (s *Scheduler) run(ctx context.Context) {
	// PANIC RECOVERY: fetcher/scraper/exporter servisləri gözlənilməz
	// panic versə (məs. üçüncü tərəf kitabxanada nadir bir edge-case),
	// bu, YALNIZ bu poll dövrəsini uğursuz edir. Ticker növbəti dövrədə
	// yenidən cəhd edəcək — proses özü ayaqda qalır. Recover olmasaydı,
	// bir dəfəlik panic bütün server prosesini (API server daxil) elə
	// bu andaca öldürərdi, çünki başladılma nöqtəsi (scheduler.Start)
	// ayrı bir goroutine-dir və orda tutulmamış panic bütün prosesi
	// dayandırır.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("scheduler: panic tutuldu, bu poll dövrəsi buraxıldı", "panic", r)
		}
	}()

	slog.Info("scheduler: poll başladı")
	start := time.Now()

	// 1. Yeni linkləri topla
	if err := s.fetcherService.FetchAll(ctx); err != nil {
		slog.Error("scheduler: FetchAll xətası", "error", err)
	}

	// 2. Yeni linkləri scrape et
	s.scraperService.ScrapeUnscraped(ctx)

	// 3. Export et
	s.exporterService.Export(ctx)

	slog.Info("scheduler: poll tamamlandı", "duration_seconds", time.Since(start).Seconds())
}
