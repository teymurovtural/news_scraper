package router

import (
	"net/http"

	"example.com/new-scraper/internal/api/handler"
	"example.com/new-scraper/internal/api/middleware"
)

// Config — router-in ehtiyac duyduğu bütün konfiqurasiyanı bir yerə
// toplayır. Əvvəllər NewRouter yalnız apiKey string qəbul edirdi — CORS və
// rate-limit əlavə olunanda parametr sayının artmasının qarşısını almaq
// üçün bunlar bir struct-a yığıldı.
type Config struct {
	APIKey string

	// CORSAllowedOrigins — boşdursa (default), CORS header-ləri heç əlavə
	// olunmur (əvvəlki davranışla eyni). Bax config.ServerConfig.CORSAllowedOrigins.
	CORSAllowedOrigins []string

	// RateLimitPerMinute — 0-dırsa, rate-limiting tamamilə deaktivdir.
	RateLimitPerMinute int
	RateLimitBurst     int
}

func NewRouter(
	itemHandler *handler.ItemHandler,
	sourceHandler *handler.SourceHandler,
	healthHandler *handler.HealthHandler,
	cfg Config,
) http.Handler {
	// protected — API_KEY tələb edən bütün "əsl" API endpoint-lər.
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/items", itemHandler.GetAll)
	protected.HandleFunc("GET /api/v1/items/{id}", itemHandler.GetByID)
	// GET /api/v1/cves — bax item_handler.go-dakı GetCVESummary şərhi:
	// bu, "kəşf" endpoint-idir (2+ mənbədə yazılan bütün CVE-lər), konkret
	// item ID-si tələb ETMİR — GET /api/v1/items/{id}-dəki related_items-dən
	// fərqli olaraq.
	protected.HandleFunc("GET /api/v1/cves", itemHandler.GetCVESummary)

	protected.HandleFunc("GET /api/v1/sources", sourceHandler.GetAll)
	protected.HandleFunc("GET /api/v1/sources/{id}", sourceHandler.GetByID)
	protected.HandleFunc("POST /api/v1/sources", sourceHandler.Create)
	protected.HandleFunc("DELETE /api/v1/sources/{id}", sourceHandler.Delete)
	protected.HandleFunc("POST /api/v1/sources/{id}/activate", sourceHandler.Activate)

	mux := http.NewServeMux()

	// BUG FIX: /view qəsdən auth-dan KƏNAR saxlanılır. Bu endpoint birbaşa
	// brauzerdə açılmaq üçün nəzərdə tutulub (bax item_handler.go-dakı şərh:
	// "http://localhost:8082/api/v1/items/{id}/view"), amma brauzer adi
	// naviqasiya zamanı custom "X-API-Key" header-i göndərə bilmir. Əvvəlki
	// versiyada API_KEY təyin olunanda /view də auth tələb edirdi, ona görə
	// brauzerdə açmaq mümkün olmurdu — endpoint-in öz məqsədi ilə ziddiyyət
	// təşkil edirdi. Qeyd: bu, /view-in məzmununu (scrape olunmuş məqalə
	// HTML-ini) API_KEY-siz də görmək mümkün etdiyi anlamına gəlir — bu,
	// şüurlu güzəşdir, çünki endpoint zatən "ictimai" saytların öz
	// məqalələrini göstərir, məxfi məlumat deyil.
	mux.HandleFunc("GET /api/v1/items/{id}/view", itemHandler.View)

	// /healthz da eyni səbəbdən (/view kimi) auth-dan KƏNAR saxlanılır:
	// Docker Compose healthcheck və gələcək orkestrasiya alətləri (k8s
	// liveness/readiness probe) X-API-Key header-i göndərmir. Cavab body-si
	// bilərəkdən minimaldır (bax health_handler.go) — heç bir konfiqurasiya
	// detalı sızdırmır, ona görə auth-suz açıq olması təhlükəsizdir.
	mux.HandleFunc("GET /healthz", healthHandler.Health)

	// Qalan bütün route-lar "/" vasitəsilə auth-lu alt-mux-a yönləndirilir.
	// Go 1.22+ ServeMux ən spesifik pattern-i seçir, ona görə yuxarıdakı
	// "GET /api/v1/items/{id}/view" bu catch-all-dan həmişə üstün olacaq.
	mux.Handle("/", middleware.APIKeyAuth(cfg.APIKey)(protected))

	// Middleware zənciri (ən xaricdən ən daxilə): CORS → rate-limit → logger → mux.
	// CORS ən xaricdə olmalıdır ki, brauzerin OPTIONS preflight sorğusu
	// auth/rate-limit-ə çatmadan cavablansın. Rate-limit logger-dən əvvəl
	// gəlir ki, rədd edilən sorğular da loglansın (görünürlük üçün).
	var h http.Handler = mux
	h = middleware.Logger(h)

	if cfg.RateLimitPerMinute > 0 {
		h = middleware.RateLimit(cfg.RateLimitPerMinute, cfg.RateLimitBurst)(h)
	}

	// CORS boş origin siyahısı ilə çağırılsa belə təhlükəsizdir — middleware
	// öz daxilində heç bir header əlavə etmədən sadəcə keçirir (bax cors.go).
	h = middleware.CORS(cfg.CORSAllowedOrigins)(h)

	return h
}
