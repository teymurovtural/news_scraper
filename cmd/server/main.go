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
	"example.com/new-scraper/internal/service/scraper/generic"

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

	// TƏHLÜKƏSİZLİK XƏBƏRDARLIĞI: API_KEY boşdursa, middleware.APIKeyAuth
	// özü sakitcə deaktiv olur (local dev üçün nəzərdə tutulub, bax
	// middleware/auth.go). Bu, produksiyada YANLIŞLIQLA unudulsa, bütün
	// /api/v1 endpoint-ləri (POST/DELETE daxil) autentifikasiyasız qalar.
	// Bunu gözdən qaçırmamaq üçün start-up zamanı AÇIQ xəbərdarlıq veririk.
	if cfg.Server.APIKey == "" {
		slog.Warn("XƏBƏRDARLIQ: API_KEY boşdur — /api/v1 endpoint-ləri (POST/DELETE daxil) HEÇ BİR autentifikasiya olmadan açıqdır. Bu yalnız local development üçün təhlükəsizdir, produksiyada mütləq API_KEY təyin edin.")
	}

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

	// DİZAYN DƏYİŞİKLİYİ: hər mənbə üçün ayrıca Go paketi (thehackernews,
	// securityweek və s.) yazmaq əvəzinə, selector/davranış konfiqurasiyası
	// scraper_configs.yaml-dan oxunur — yeni "normal" bir sayt əlavə etmək
	// üçün YENİ KOD YAZMAĞA EHTİYAC QALMIR (bax internal/service/scraper/generic).
	// Köhnə 6 sayta-xas paket hələ də repoda saxlanılıb (referans üçün),
	// amma artıq buradan import edilmir.
	scraperConfigs, err := generic.LoadConfigs("scraper_configs.yaml")
	if err != nil {
		slog.Error("scraper konfiqurasiyaları yüklənmədi", "error", err)
		os.Exit(1)
	}
	scrapers := generic.BuildScrapers(pw, cfg.Playwright.Headless, scraperConfigs)
	baseURL := fmt.Sprintf("http://localhost:%s", cfg.Server.Port)
	scraperService := scraper.NewScraperService(feedItemRepo, sourceRepo, scrapers, cfg.Poller.WorkerCount, baseURL)

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
	healthHandler := handler.NewHealthHandler(db)

	r := router.NewRouter(itemHandler, sourceHandler, healthHandler, router.Config{
		APIKey:             cfg.Server.APIKey,
		CORSAllowedOrigins: cfg.Server.CORSAllowedOrigins,
		RateLimitPerMinute: cfg.Server.RateLimitPerMinute,
		RateLimitBurst:     cfg.Server.RateLimitBurst,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sch.Start(ctx)

	addr := fmt.Sprintf(":%s", cfg.Server.Port)

	// BUG FIX (Slowloris / resurs tükənməsi): əvvəlki versiyada http.Server-in
	// heç bir timeout-u yox idi — yavaş/zərərli bir client connection-u
	// sonsuz açıq saxlaya, server-in fayl deskriptor/goroutine resurslarını
	// tükədə bilərdi. Bu dörd timeout standart Go müdafiəsidir:
	//   - ReadHeaderTimeout: header-lərin oxunması üçün maksimum vaxt
	//   - ReadTimeout: bütün request body-nin oxunması üçün maksimum vaxt
	//   - WriteTimeout: cavabın yazılması üçün maksimum vaxt (/view kimi
	//     bəzi cavablar adi JSON-dan bir az böyük ola bilər, ona görə bir az
	//     səxavətli saxlanılıb)
	//   - IdleTimeout: keep-alive connection-un fəaliyyətsiz qala biləcəyi
	//     maksimum vaxt
	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
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

	// BUG FIX (graceful shutdown): əvvəlki versiya sch.Stop()-u çağırıb
	// DƏRHAL server.Shutdown-a keçirdi, sonra funksiya qayıdanda defer-lər
	// (pw.Stop(), db.Close()) işə düşürdü — scheduler-in daxili goroutine-i
	// hələ də (uzun sürən) bir scrape/export mərhələsinin ortasında ola
	// bilərdi. Nəticədə DB pool/Playwright browser hələ istifadə olunarkən
	// bağlana bilərdi.
	//
	// İndi əvvəlcə əsas context-i ləğv edirik (bu, fetcher.FetchSource
	// daxilindəki fetchCtx kimi context-ə bağlı şəbəkə sorğularının dərhal
	// kəsilməsinə səbəb olur), sonra sch.Stop() ilə yeni dövrənin
	// başlamasının qarşısını alırıq, sonra da Wait(timeout) ilə davam edən
	// dövrənin HƏQİQƏTƏN bitməsini gözləyirik.
	//
	// QEYD: Playwright scrape mərhələsi hazırda context-ə bağlı deyil (bax
	// scheduler.Wait şərhi) — ona görə bu gözləmə timeout-a çata bilər, əgər
	// düz o an bir scrape dövrü gedirsə. Bu halda xəbərdarlıq loglanır və
	// shutdown YENƏ DƏ davam edir (server prosesi bağlanacaq).
	cancel()
	sch.Stop()

	const schedulerShutdownTimeout = 30 * time.Second
	if !sch.Wait(schedulerShutdownTimeout) {
		slog.Warn("scheduler: gözləmə vaxtı bitdi, arxa planda bir poll dövrü hələ davam edir ola bilər — buna baxmayaraq shutdown davam edir",
			"timeout", schedulerShutdownTimeout.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown xətası", "error", err)
	}

	slog.Info("server dayandı")
}
