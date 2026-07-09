package router

import (
	"net/http"

	"example.com/new-scraper/internal/api/handler"
	"example.com/new-scraper/internal/api/middleware"
)

func NewRouter(
	itemHandler *handler.ItemHandler,
	sourceHandler *handler.SourceHandler,
	apiKey string,
) http.Handler {
	// protected — API_KEY tələb edən bütün "əsl" API endpoint-lər.
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/items", itemHandler.GetAll)
	protected.HandleFunc("GET /api/v1/items/{id}", itemHandler.GetByID)

	protected.HandleFunc("GET /api/v1/sources", sourceHandler.GetAll)
	protected.HandleFunc("GET /api/v1/sources/{id}", sourceHandler.GetByID)
	protected.HandleFunc("POST /api/v1/sources", sourceHandler.Create)

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

	// Qalan bütün route-lar "/" vasitəsilə auth-lu alt-mux-a yönləndirilir.
	// Go 1.22+ ServeMux ən spesifik pattern-i seçir, ona görə yuxarıdakı
	// "GET /api/v1/items/{id}/view" bu catch-all-dan həmişə üstün olacaq.
	mux.Handle("/", middleware.APIKeyAuth(apiKey)(protected))

	return middleware.Logger(mux)
}
