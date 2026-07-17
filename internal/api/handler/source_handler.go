package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"example.com/new-scraper/internal/domain"
)

type SourceHandler struct {
	sourceRepo domain.SourceRepository
}

func NewSourceHandler(sourceRepo domain.SourceRepository) *SourceHandler {
	return &SourceHandler{sourceRepo: sourceRepo}
}

func (h *SourceHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	sources, err := h.sourceRepo.GetAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mənbələr alınmadı")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sources": sources,
	})
}

func (h *SourceHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "yanlış ID")
		return
	}

	source, err := h.sourceRepo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrSourceNotFound) {
			writeError(w, http.StatusNotFound, "mənbə tapılmadı")
			return
		}
		writeError(w, http.StatusInternalServerError, "mənbə alınmadı")
		return
	}

	writeJSON(w, http.StatusOK, source)
}

// Delete — mənbəni deaktiv edir (soft delete, bax domain.SourceRepository.
// Deactivate). Uğurlu olsa 204 No Content qaytarır (silinən/dəyişən
// resurs üçün body yoxdur), mənbə tapılmasa 404.
func (h *SourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "yanlış ID")
		return
	}

	if err := h.sourceRepo.Deactivate(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrSourceNotFound) {
			writeError(w, http.StatusNotFound, "mənbə tapılmadı")
			return
		}
		writeError(w, http.StatusInternalServerError, "mənbə deaktiv edilmədi")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Activate — Delete-in (soft-delete) əksi. Ya əl ilə DELETE edilmiş bir
// mənbəni geri qaytarmaq üçün, ya da IncrementFailCount-un 20-lik həddinə
// görə AVTOMATİK deaktiv olmuş bir mənbəni yenidən aktivləşdirmək üçün
// istifadə olunur (bax scraper_service.go/fetcher.go-dakı "XƏBƏRDARLIQ —
// mənbə ... avtomatik deaktiv edildi" log sətri — bu, məhz həmin mənbəni
// geri qaytarmaq üçün endpoint-dir). Uğurlu olsa 204, mənbə tapılmasa 404.
func (h *SourceHandler) Activate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "yanlış ID")
		return
	}

	if err := h.sourceRepo.Activate(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrSourceNotFound) {
			writeError(w, http.StatusNotFound, "mənbə tapılmadı")
			return
		}
		writeError(w, http.StatusInternalServerError, "mənbə aktivləşdirilmədi")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// maxCreateBodySize — POST /sources body-si üçün yuxarı limit. Limitsiz
// oxunan body server-i (yaddaş baxımından) DoS-a açıq edir.
const maxCreateBodySize = 1 << 20 // 1 MB

func (h *SourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	// BUG FIX: əvvəlki versiya r.Body-ni limitsiz oxuyurdu — ölçüsüz böyük
	// bir body server yaddaşını doldura bilərdi. http.MaxBytesReader bu
	// limitdən sonrakı oxuma cəhdlərində xəta qaytarır.
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateBodySize)

	var input struct {
		Name         string `json:"name"`
		SiteURL      string `json:"site_url"`
		FeedURL      string `json:"feed_url"`
		Category     string `json:"category"`
		PollInterval int    `json:"poll_interval"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "yanlış məlumat formatı (və ya body 1MB limitini keçib)")
		return
	}

	if input.Name == "" || input.FeedURL == "" || input.SiteURL == "" {
		writeError(w, http.StatusBadRequest, "name, site_url və feed_url mütləq doldurulmalıdır")
		return
	}

	// BUG FIX (Path Traversal): source.Name sonradan exporter.go tərəfindən
	// fayl sistemi qovluq adı kimi istifadə olunur — validasiya olmasa,
	// "../"  kimi ardıcıllıqlar exports/ qovluğundan kənara çıxa bilərdi.
	// Ətraflı izah üçün bax: url_validation.go — validateSourceName.
	if err := validateSourceName(input.Name); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("name yanlışdır: %v", err))
		return
	}

	// BUG FIX (SSRF): feed_url/site_url server tərəfindən sonradan özü fetch
	// ediləcək (fetcher.go, scraper-lər) — validasiya olmasa, daxili şəbəkə
	// ünvanlarına (localhost, DB portu, bulud metadata ünvanı və s.) sorğu
	// göndərtmək mümkün olardı. Ətraflı izah üçün bax: url_validation.go.
	if err := validatePublicHTTPURL(input.FeedURL); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("feed_url yanlışdır: %v", err))
		return
	}
	if err := validatePublicHTTPURL(input.SiteURL); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("site_url yanlışdır: %v", err))
		return
	}

	pollInterval := input.PollInterval
	if pollInterval <= 0 {
		pollInterval = 900 // default: 15 dəqiqə
	}

	source := &domain.Source{
		Name:         input.Name,
		SiteURL:      input.SiteURL,
		FeedURL:      input.FeedURL,
		Category:     input.Category,
		PollInterval: pollInterval,
	}

	if err := h.sourceRepo.Create(r.Context(), source); err != nil {
		if errors.Is(err, domain.ErrDuplicateSource) {
			writeError(w, http.StatusConflict, "bu mənbə artıq mövcuddur")
			return
		}
		writeError(w, http.StatusInternalServerError, "mənbə əlavə edilmədi")
		return
	}

	writeJSON(w, http.StatusCreated, source)
}
