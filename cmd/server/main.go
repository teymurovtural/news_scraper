package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/new-scraper/internal/api/handler"
	"example.com/new-scraper/internal/api/router"
	"example.com/new-scraper/internal/config"
	"example.com/new-scraper/internal/platform/database"
	loggerpkg "example.com/new-scraper/internal/platform/logger"
	"example.com/new-scraper/internal/repository"
	"example.com/new-scraper/internal/scheduler"
	"example.com/new-scraper/internal/service/exporter"
	"example.com/new-scraper/internal/service/fetcher"
	"example.com/new-scraper/internal/service/scraper"
	"example.com/new-scraper/internal/service/scraper/sources/bleepingcomputer"
	"example.com/new-scraper/internal/service/scraper/sources/cyberscoop"
	"example.com/new-scraper/internal/service/scraper/sources/darkreading"
	"example.com/new-scraper/internal/service/scraper/sources/itsecurityguru"
	"example.com/new-scraper/internal/service/scraper/sources/securityweek"
	"example.com/new-scraper/internal/service/scraper/sources/thehackernews"

	"github.com/playwright-community/playwright-go"
)

func main() {
	// QEYD: config yüklənməmişdən əvvəl LOG_LEVEL/LOG_FORMAT hələ məlum
	// deyil, ona görə bu tək sətir üçün standart "log" paketini saxlayırıq.
	// Config yükləndikdən sonra bütün tətbiq slog-a keçir.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	lg := loggerpkg.New(cfg.Log)
	slog.SetDefault(lg)

	slog.Info("konfiqurasiya yükləndi", "log_level", cfg.Log.Level, "log_format", cfg.Log.Format)

	db, err := database.NewPostgresDB(cfg.DB.ConnectionString())
	if err != nil {
		slog.Error("database bağlantısı qurulmadı", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("database bağlantısı quruldu")

	pw, err := playwright.Run()
	if err != nil {
		slog.Error("playwright başladılmadı", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := pw.Stop(); err != nil {
			slog.Error("playwright dayandırılarkən xəta", "error", err)
		}
	}()
	slog.Info("playwright başladı")

	sourceRepo := repository.NewSourceRepository(db)
	feedItemRepo := repository.NewFeedItemRepository(db)

	fetcherService := fetcher.NewFetcherService(sourceRepo, feedItemRepo)

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

	exporterService := exporter.NewExporterService(sourceRepo, feedItemRepo)

	sch := scheduler.NewScheduler(
		fetcherService,
		scraperService,
		exporterService,
		cfg.Poller.IntervalSeconds,
		cfg.Poller.WorkerCount,
	)

	itemHandler := handler.NewItemHandler(feedItemRepo)
	sourceHandler := handler.NewSourceHandler(sourceRepo)

	r := router.NewRouter(itemHandler, sourceHandler, cfg.Server.APIKey)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sch.Start(ctx)

	addr := fmt.Sprintf(":%s", cfg.Server.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		slog.Info("server başladı", "addr", "http://localhost"+addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server xətası", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("server dayandırılır...")
	sch.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown xətası", "error", err)
	}

	slog.Info("server dayandı")
}
