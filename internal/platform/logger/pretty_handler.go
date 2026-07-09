package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// prettyHandler — LOG_FORMAT=pretty üçün custom slog.Handler.
//
// slog standart kitabxanada yalnız JSONHandler və TextHandler ilə gəlir,
// ikisi də rəng/emoji dəstəkləmir. Bu handler hər sətrə səviyyəyə görə
// emoji (✅/⚠️/❌) və ANSI rəng kodu əlavə edir — inkişaf zamanı terminalda
// bir baxışda uğurlu/xəbərdarlıq/xəta ayırd etmək üçün.
//
// Qeyd: ANSI rəng kodları müasir Windows Terminal/PowerShell 7+ tərəfindən
// dəstəklənir. Köhnə cmd.exe-də kodlar rənglənmədən görünə bilər (\033[31m
// kimi ham mətn) — bu halda LOG_FORMAT=text istifadə et.
type prettyHandler struct {
	mu    *sync.Mutex
	out   io.Writer
	level slog.Leveler
	attrs []slog.Attr
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorGray   = "\033[90m"
)

func newPrettyHandler(w io.Writer, level slog.Leveler) *prettyHandler {
	return &prettyHandler{mu: &sync.Mutex{}, out: w, level: level}
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	var emoji, color string
	switch {
	case r.Level >= slog.LevelError:
		emoji, color = "❌", colorRed
	case r.Level >= slog.LevelWarn:
		emoji, color = "⚠️ ", colorYellow
	case r.Level >= slog.LevelInfo:
		emoji, color = "✅", colorGreen
	default: // Debug
		emoji, color = "🐛", colorGray
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s %s%s  %s",
		color, emoji, r.Time.Format("15:04:05"), colorReset, r.Message)

	// Handler-ə əvvəlcədən bağlanmış attrs (WithAttrs vasitəsilə)
	for _, a := range h.attrs {
		fmt.Fprintf(&b, " %s%s=%s", colorGray, a.Key, colorReset)
		fmt.Fprintf(&b, "%v", a.Value.Any())
	}
	// Bu konkret record-un öz attrs-ları (slog.Info("msg", "key", val) hissəsi)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s%s=%s", colorGray, a.Key, colorReset)
		fmt.Fprintf(&b, "%v", a.Value.Any())
		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprintln(h.out, b.String())
	return err
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	newAttrs = append(newAttrs, h.attrs...)
	newAttrs = append(newAttrs, attrs...)
	return &prettyHandler{mu: h.mu, out: h.out, level: h.level, attrs: newAttrs}
}

// WithGroup — bu layihədə slog group-ları istifadə olunmur, ona görə
// sadəcə özünü qaytarır (group adını nəzərə almadan).
func (h *prettyHandler) WithGroup(_ string) slog.Handler {
	return h
}
