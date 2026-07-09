package database

import (
	"context"
	"fmt"
	"log/slog"
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
	pool, err := pgxpool.New(context.Background(), dsn)
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
