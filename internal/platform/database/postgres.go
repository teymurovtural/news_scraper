package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BUG FIX: əvvəlki versiya Ping-i yalnız 1 dəfə sınayırdı. `docker compose up -d`
// dərhal qayıdır, amma konteynerin İÇİNDƏKİ Postgres prosesi hələ bir neçə
// saniyə hazır olmaya bilər (xüsusən ilk dəfə volume yaradılanda). Bu zaman
// pəncərəsində tətbiqi işə salsan, "server əlçatmazdır" xətası ilə dərhal
// çökür (cmd/server/main.go log.Fatal edir). İndi bir neçə cəhd, aralarında
// qısa gözləmə ilə sınanılır.
const (
	maxPingRetries = 10
	pingRetryDelay = 2 * time.Second
)

func NewPostgresDB(dsn string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("database: DSN parse edilmədi: %w", err)
	}

	// BUG FIX (pool tənzimlənməsi): əvvəlki versiya pgxpool.New(dsn) çağırırdı
	// — bu, pgx-in default MaxConns dəyərini (CPU sayına bağlı, adətən 4)
	// istifadə edirdi. WORKER_COUNT artırılanda (paralel scraper worker-ləri,
	// hər biri UpdateScrapedData ilə DB-yə yazır) bu default, iş yükü ilə
	// uyğunlaşdırılmadan qala bilərdi. DB_MAX_CONNS env dəyişəni ilə indi
	// açıq tənzimlənə bilər — təyin olunmasa, pgx-in öz default-u qalır
	// (davranış DƏYİŞMİR, sadəcə İSTƏYƏ BAĞLI override imkanı əlavə olunur).
	if v := os.Getenv("DB_MAX_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			poolCfg.MaxConns = int32(n)
		} else {
			slog.Warn("database: DB_MAX_CONNS yanlışdır, pgx default-u istifadə olunur", "value", v)
		}
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: bağlantı açılmadı: %w", err)
	}

	var pingErr error
	for attempt := 1; attempt <= maxPingRetries; attempt++ {
		pingErr = pool.Ping(context.Background())
		if pingErr == nil {
			return pool, nil
		}

		if attempt < maxPingRetries {
			slog.Warn("database: Postgres hələ hazır deyil",
				"attempt", attempt, "max_attempts", maxPingRetries,
				"error", pingErr, "retry_delay", pingRetryDelay.String(),
			)
			time.Sleep(pingRetryDelay)
		}
	}

	pool.Close()
	return nil, fmt.Errorf("database: server %d cəhddən sonra əlçatmazdır: %w", maxPingRetries, pingErr)
}
