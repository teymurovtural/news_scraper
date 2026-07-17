package handler

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"example.com/new-scraper/internal/domain"
)

type ItemHandler struct {
	feedItemRepo domain.FeedItemRepository
}

func NewItemHandler(feedItemRepo domain.FeedItemRepository) *ItemHandler {
	return &ItemHandler{feedItemRepo: feedItemRepo}
}

func (h *ItemHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	items, err := h.feedItemRepo.GetAll(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "xəbərlər alınmadı")
		return
	}

	total, err := h.feedItemRepo.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "xəbər sayı alınmadı")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// itemResponse — GET /api/v1/items/{id} cavabı. domain.FeedItem-in bütün
// sahələrini (embedded struct vasitəsilə, JSON-da "flatten" olunur) daşıyır,
// üstünə YALNIZ CVE-ID-si olan məqalələr üçün related_items əlavə edir.
type itemResponse struct {
	domain.FeedItem
	RelatedItems []domain.RelatedFeedItem `json:"related_items,omitempty"`
}

func (h *ItemHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "yanlış ID")
		return
	}

	item, err := h.feedItemRepo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrItemNotFound) {
			writeError(w, http.StatusNotFound, "xəbər tapılmadı")
			return
		}
		writeError(w, http.StatusInternalServerError, "xəbər alınmadı")
		return
	}

	response := itemResponse{FeedItem: *item}

	// Yalnız CVE-ID-si olan məqalələr üçün əlaqələndirmə sorğusu edirik —
	// digərlərində bu, həmişə boş nəticə verəcək, lazımsız DB sorğusudur.
	if len(item.CVEIDs) > 0 {
		related, err := h.feedItemRepo.GetRelatedByCVE(r.Context(), item.CVEIDs, item.ID, 10)
		if err != nil {
			// Əlaqəli məqalələr "nice-to-have" məlumatdır — bu sorğu
			// uğursuz olsa belə, əsas item-i qaytarmaqdan imtina etmirik,
			// sadəcə related_items boş qalır.
			slog.Error("item_handler: əlaqəli məqalələr alınmadı", "item_id", id, "error", err)
		} else {
			response.RelatedItems = related
		}
	}

	writeJSON(w, http.StatusOK, response)
}

const viewPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>TITLE_PLACEHOLDER</title>
<style>
  * { box-sizing: border-box; }
  body { max-width: 860px; margin: 40px auto; font-family: -apple-system, Arial, sans-serif; line-height: 1.7; padding: 0 24px; color: #1a1a1a; background: #fff; }
  img { max-width: 100%; height: auto; display: block; margin: 16px auto; float: none !important; }
  figure { margin: 16px 0; text-align: center; }
  figcaption { font-size: 13px; color: #666; margin-top: 6px; }
  .separator { clear: both; overflow: hidden; margin: 12px 0; }
  p { margin: 0 0 14px; }
  h1.article-title { font-size: 28px; margin-bottom: 6px; line-height: 1.3; }
  h2, h3, h4 { color: #111; margin: 20px 0 10px; }
  .meta { color: #555; margin-bottom: 6px; font-size: 14px; }
  .source { color: #888; margin-bottom: 28px; font-size: 13px; }
  a { color: #0066cc; }
  hr { margin: 32px 0; border: none; border-top: 1px solid #e0e0e0; }
  blockquote { border-left: 3px solid #ddd; margin: 16px 0; padding: 8px 16px; color: #444; }
  pre, code { background: #f5f5f5; border-radius: 4px; font-size: 13px; padding: 2px 6px; }
  pre { padding: 12px 16px; overflow-x: auto; }
  iframe { width: 100%; aspect-ratio: 16 / 9; height: auto; border: none; border-radius: 4px; margin: 16px 0; }
</style>
</head>
<body>
  <h1 class="article-title">H1_TITLE_PLACEHOLDER</h1>
  <div class="meta">META_PLACEHOLDER</div>
  <div class="source"><a href="LINK_PLACEHOLDER" target="_blank">Orijinal: LINKTEXT_PLACEHOLDER</a></div>
  <hr>
  CONTENT_PLACEHOLDER
</body>
</html>`

// View — feed_item-in content_html-ni birbaşa brauzerdə render olunan
// HTML səhifə kimi qaytarır (JSON yox). Brauzerdə açmaq üçün:
//
//	http://localhost:8082/api/v1/items/{id}/view
func (h *ItemHandler) View(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "yanlış ID", http.StatusBadRequest)
		return
	}

	item, err := h.feedItemRepo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrItemNotFound) {
			http.Error(w, "xəbər tapılmadı", http.StatusNotFound)
			return
		}
		http.Error(w, "xəbər alınmadı", http.StatusInternalServerError)
		return
	}

	if item.ContentHTML == "" {
		http.Error(w, "bu xəbər hələ scrape edilməyib (content_html boşdur)", http.StatusNotFound)
		return
	}

	// BUG FIX: əvvəlki versiya "TITLE_PLACEHOLDER" və "TITLE_PLACEHOLDER2"
	// açarlarını istifadə edirdi. strings.NewReplacer hər mövqedə siyahıdakı
	// İLK uyğun gələn açarı işlədir — "TITLE_PLACEHOLDER" "TITLE_PLACEHOLDER2"-nin
	// prefiksi olduğu üçün, <h1> tag-ındakı yer "TITLE_PLACEHOLDER" kimi tanınıb
	// əvəz olunurdu, sondakı "2" isə toxunulmadan qalırdı — nəticədə hər başlığın
	// sonuna səhvən "2" yapışırdı (məs. "...insults to Trump" → "...insults to Trump2").
	// Yeni açar ("H1_TITLE_PLACEHOLDER") heç bir başqa açarla prefiks toqquşması
	// yaratmır.
	page := strings.NewReplacer(
		"TITLE_PLACEHOLDER", html.EscapeString(item.Title),
		"H1_TITLE_PLACEHOLDER", html.EscapeString(item.Title),
		"META_PLACEHOLDER", html.EscapeString(item.Author)+" — "+html.EscapeString(item.PublishedDate),
		"LINK_PLACEHOLDER", html.EscapeString(item.Link),
		"LINKTEXT_PLACEHOLDER", html.EscapeString(item.Link),
		"CONTENT_PLACEHOLDER", item.ContentHTML,
	).Replace(viewPageTemplate)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(page))
}

// GetCVESummary — GET /api/v1/cves. YALNIZ 2+ məqalədə keçən (yəni
// həqiqətən əlaqələndirmə mənası olan) CVE-lərin siyahısını, hər birinin
// məqalələri ilə birlikdə qaytarır. Bu, "kəşf" endpoint-idir — GetByID-dəki
// related_items-dən (konkret bir item ID-si tələb edir) fərqli olaraq,
// heç bir ID bilmədən "hansı hadisələr birdən çox mənbədə yazılıb?"
// sualına birbaşa cavab verir.
func (h *ItemHandler) GetCVESummary(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.feedItemRepo.GetCVESummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CVE siyahısı alınmadı")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cves":  summaries,
		"total": len(summaries),
	})
}

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 20
	offset = 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	return limit, offset
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("JSON encode xətası", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
