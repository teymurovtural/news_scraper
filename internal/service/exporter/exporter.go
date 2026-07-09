package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/new-scraper/internal/domain"
)

type ExportItem struct {
	ID            int64              `json:"id"`
	Title         string             `json:"title"`
	Link          string             `json:"link"`
	Author        string             `json:"author"`
	PublishedDate string             `json:"published_date"`
	Content       string             `json:"content"`
	ContentHTML   string             `json:"content_html,omitempty"`
	ViewURL       string             `json:"view_url,omitempty"`
	Images        []domain.ImageItem `json:"images"`
	VideoURL      string             `json:"video_url,omitempty"`
	PublishedAt   string             `json:"published_at"`
	ScrapedAt     string             `json:"scraped_at"`
}

type ExporterService struct {
	sourceRepo   domain.SourceRepository
	feedItemRepo domain.FeedItemRepository
}

func NewExporterService(
	sourceRepo domain.SourceRepository,
	feedItemRepo domain.FeedItemRepository,
) *ExporterService {
	return &ExporterService{
		sourceRepo:   sourceRepo,
		feedItemRepo: feedItemRepo,
	}
}

func (s *ExporterService) Export(ctx context.Context) {
	sources, err := s.sourceRepo.GetAll(ctx)
	if err != nil {
		slog.Error("exporter: mənbələr alınmadı", "error", err)
		return
	}

	for _, source := range sources {
		// Son export vaxtını DB-dən al
		var after time.Time
		if source.LastExportedAt != nil {
			after = *source.LastExportedAt
		}

		// Yalnız son exportdan sonra scrape olunanları çək
		items, err := s.feedItemRepo.GetBySourceAfterScrapedAt(ctx, source.ID, after)
		if err != nil {
			slog.Error("exporter: xəbərlər alınmadı", "source", source.Name, "error", err)
			continue
		}

		if len(items) == 0 {
			continue
		}

		var exportItems []ExportItem
		for _, item := range items {
			e := ExportItem{
				ID:            item.ID,
				Title:         item.Title,
				Link:          item.Link,
				Author:        item.Author,
				PublishedDate: item.PublishedDate,
				Content:       item.Content,
				ContentHTML:   item.ContentHTML,
				ViewURL:       item.ViewURL,
				Images:        item.Images,
				VideoURL:      item.VideoURL,
			}
			if item.PublishedAt != nil {
				e.PublishedAt = item.PublishedAt.Format(time.RFC3339)
			}
			if item.ScrapedAt != nil {
				e.ScrapedAt = item.ScrapedAt.Format(time.RFC3339)
			}
			exportItems = append(exportItems, e)
		}

		dirName := strings.ToLower(strings.ReplaceAll(source.Name, " ", "_"))
		dirPath := fmt.Sprintf("exports/%s", dirName)

		// TƏHLÜKƏSİZLİK QEYDİ (Path Traversal — 2-ci qat): source.Name
		// artıq source_handler.Create-də validasiya olunur (bax
		// validateSourceName), ona görə normal axında "../" bura heç vaxt
		// çatmamalıdır. Amma bu, ikinci, MÜSTƏQİL bir sərhəddir: məsələn
		// DB-yə əl ilə (migration, backfill script və s.) yazılmış köhnə
		// bir sətir, ya da gələcəkdə əlavə olunacaq başqa bir kod yolu
		// (import, admin script) həmin validasiyanı yan keçə bilər.
		// isPathInsideExports faktiki hesablanmış yolun, simvolik
		// linklər/".." nə olursa olsun, HƏMİŞƏ "exports/" kökü daxilində
		// qaldığını təsdiqləyir — əks halda həmin mənbə üçün export
		// tamamilə atlanılır, heç bir fayl yazılmır.
		if !isPathInsideExports(dirPath) {
			slog.Error("exporter: təhlükəsiz olmayan export yolu, keçilir", "source", source.Name, "dir", dirPath)
			continue
		}

		if err := os.MkdirAll(dirPath, 0755); err != nil {
			slog.Error("exporter: qovluq yaradılmadı", "dir", dirPath, "error", err)
			continue
		}

		date := time.Now().Format("2006-01-02")
		fileName := fmt.Sprintf("%s/export_%s.json", dirPath, date)

		added, err := appendToFile(fileName, exportItems)
		if err != nil {
			slog.Error("exporter: fayl yazılmadı", "source", source.Name, "file", fileName, "error", err)
			continue
		}

		// DB-də last_exported_at-i yenilə
		if err := s.sourceRepo.UpdateLastExportedAt(ctx, source.ID); err != nil {
			slog.Error("exporter: last_exported_at yenilənmədi", "source", source.Name, "error", err)
		}

		slog.Info("exporter: export edildi", "source", source.Name, "file", fileName, "new_items", added)
	}
}

// isPathInsideExports — verilmiş qovluq yolunun, bütün "../" və simvolik
// linklər həll olunduqdan sonra, faktiki olaraq "exports/" kökünün daxilində
// qaldığını yoxlayır. Path Traversal-a qarşı son (2-ci qat) sərhəddir —
// bax yuxarıdakı çağırış yerindəki qeyd.
func isPathInsideExports(dirPath string) bool {
	absExports, err := filepath.Abs("exports")
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absExports, absDir)
	if err != nil {
		return false
	}

	// rel "../..." ilə başlayırsa, ya da tam olaraq ".."-dirsə, dirPath
	// exports/ kökündən kənara çıxıb deməkdir.
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// appendToFile — mövcud JSON faylına yeni xəbərlər əlavə edir və faktiki
// əlavə olunan xəbər sayını qaytarır.
//
// BUG FIX 1 (dublikat riski): əvvəlki versiya yoxlamadan bütün `items`-i
// fayla əlavə edirdi. Əgər `UpdateLastExportedAt` DB-yə yazılmazdan əvvəl
// proses çökmüşdüsə (crash, restart və s.), DB-də köhnə `last_exported_at`
// qalırdı və növbəti export dövründə eyni xəbərlər YENİDƏN "yeni" sayılıb
// fayla ikinci dəfə yazılırdı. İndi hər item `Link`-ə görə yoxlanılır — fayldakı
// link artıq varsa, təkrar əlavə olunmur, ona görə bu proses idempotentdir.
//
// Qeyd: dedup açarı olaraq `ID` yox, `Link` istifadə olunur — ID Postgres-in
// auto-increment sütunudur və yalnız BİR DB "ömrü" daxilində unikaldır (DB
// sıfırlansa/yenidən yaradılsa, ID-lər yenidən 1-dən başlayır və köhnə
// export fayllarındakı ID-lərlə təsadüfən üst-üstə düşə bilər). `Link` isə
// DB-də də UNIQUE constraint daşıyan, əsl, dəyişməz identifikatordur.
//
// BUG FIX 2 (yarımçıq fayl riski): əvvəlki versiya birbaşa hədəf fayla yazırdı
// (`os.Create` + `Encode`) — yazma zamanı proses kəsilsə (crash, disk dolması
// və s.), fayl yarımçıq/korlanmış JSON vəziyyətdə qala bilərdi. İndi əvvəlcə
// müvəqqəti fayla ("*.tmp") yazılır, disk-ə sync edilir, sonra hədəf faylın
// üzərinə ATOMIK `rename` edilir. Eyni fayl sistemində rename atomikdir —
// crash olsa, ya köhnə fayl bütöv qalır, ya da tam yeni fayl görünür.
func appendToFile(fileName string, items []ExportItem) (int, error) {
	var existing []ExportItem
	seen := make(map[string]bool)

	data, err := os.ReadFile(fileName)
	if err == nil {
		if uerr := json.Unmarshal(data, &existing); uerr != nil {
			return 0, fmt.Errorf("mövcud fayl JSON kimi oxunmadı: %w", uerr)
		}
	}
	for _, it := range existing {
		seen[it.Link] = true
	}

	added := 0
	for _, it := range items {
		if seen[it.Link] {
			continue
		}
		existing = append(existing, it)
		seen[it.Link] = true
		added++
	}

	if added == 0 {
		return 0, nil
	}

	tmpFile := fileName + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return 0, fmt.Errorf("müvəqqəti fayl yaradılmadı: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(existing); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return 0, fmt.Errorf("JSON encode edilmədi: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return 0, fmt.Errorf("fayl disk-ə yazılmadı: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpFile)
		return 0, fmt.Errorf("fayl bağlanmadı: %w", err)
	}

	if err := os.Rename(tmpFile, fileName); err != nil {
		os.Remove(tmpFile)
		return 0, fmt.Errorf("fayl əvəz edilmədi: %w", err)
	}

	return added, nil
}
