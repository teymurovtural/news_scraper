package handler

import (
	"context"
	"net/http"
	"time"
)

// Pinger — DB bağlantısının canlı olduğunu yoxlamaq üçün minimal interfeys.
// *pgxpool.Pool bu metodu artıq təmin edir (strukturaldır, əlavə uyğunlaşdırma
// lazım deyil). Ayrıca kiçik interfeys olması, HealthHandler-i testlərdə
// real DB olmadan (saxta bir Pinger ilə) sınamağa imkan verir.
type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	db Pinger
}

func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// pingTimeout — DB ping-i üçün yuxarı vaxt limiti. Orkestrasiya alətləri
// (Docker healthcheck, k8s liveness/readiness probe) adətən qısa müddətdə
// cavab gözləyir — DB "asılıb qalıbsa", bu endpoint də əbədi gözləməməlidir.
const pingTimeout = 3 * time.Second

// Health — GET /healthz. Auth TƏLƏB ETMİR (bax router.go-dakı qeyd) — Docker/
// k8s health-check mexanizmləri X-API-Key header-i göndərmir. Cavab body-si
// bilərəkdən minimaldır (yalnız "ok"/"unhealthy" statusu) — DB host/parol
// kimi heç bir konfiqurasiya detalı açılmır, çünki bu endpoint auth-suz
// hər kəsə açıqdır.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
