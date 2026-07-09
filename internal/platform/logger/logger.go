package logger

import (
	"log/slog"
	"os"
	"strings"

	"example.com/new-scraper/internal/config"
)

// New — bütün tətbiq üçün TƏK bir *slog.Logger qurur.
//
// NİYƏ slog: Go 1.21+ standart kitabxanasında gəlir, yeni dependency lazım
// deyil. Əvvəlki `log.Printf("scraper_service: ✅ [%d] %s", ...)` kimi sadə
// mətn logları əvəzinə, hər log yazısı STRUCTURED sahələrlə gəlir (məs.
// `slog.Int("item_id", 671)`) — bu, kodun məntiqinə TOXUNMADAN, sadəcə
// LOG_FORMAT env dəyişəni ilə üç fərqli çıxış formatına imkan verir:
//
//   - LOG_FORMAT=json → log toplayıcı alətlər üçün (Grafana Loki, ELK və s.):
//     {"time":"...","level":"INFO","msg":"scrape uğurlu","source":"thehackernews","item_id":671}
//   - LOG_FORMAT=text → sadə, rəngsiz mətn (production-a yaxın, amma oxunaqlı):
//     time=2026-07-09T12:00:00.000+04:00 level=INFO msg="scrape uğurlu" source=thehackernews item_id=671
//   - LOG_FORMAT=pretty → development üçün, emoji + ANSI rəng (bax pretty_handler.go):
//     ✅ 12:00:00  scrape uğurlu source=thehackernews item_id=671
//
// LOG_LEVEL (debug/info/warn/error) hər üç formatda eyni cür işləyir —
// təyin olunan səviyyədən aşağı olan loglar (məs. LOG_LEVEL=warn olanda
// Info/Debug) heç yazılmır.
func New(cfg config.LogConfig) *slog.Logger {
	level := parseLevel(cfg.Level)

	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	case "pretty":
		handler = newPrettyHandler(os.Stdout, level)
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	return slog.New(handler)
}

// parseLevel — LOG_LEVEL env dəyərini slog.Level-ə çevirir. Tanınmayan/boş
// dəyər üçün təhlükəsiz default olaraq Info qaytarır (heç vaxt xəta ilə
// tətbiqi dayandırmır — logging konfiqurasiyası kritik başlanğıc şərti
// deyil).
func parseLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
