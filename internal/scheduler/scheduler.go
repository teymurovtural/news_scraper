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
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	slog.Info("scheduler başladı", "interval", s.interval.String(), "worker_count", s.workerCount)

	go func() {
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

func (s *Scheduler) Stop() {
	close(s.stopCh)
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
