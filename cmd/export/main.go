package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"example.com/new-scraper/internal/config"
	"example.com/new-scraper/internal/platform/database"
	"example.com/new-scraper/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.NewPostgresDB(cfg.DB.ConnectionString())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	sourceRepo := repository.NewSourceRepository(db)
	feedItemRepo := repository.NewFeedItemRepository(db)

	ctx := context.Background()

	sources, err := sourceRepo.GetAll(ctx)
	if err != nil {
		log.Fatal(err)
	}

	date := time.Now().Format("2006-01-02")

	for _, source := range sources {
		items, err := feedItemRepo.GetBySource(ctx, source.ID, 10000, 0)
		if err != nil {
			log.Printf("❌ [%s] xəbərlər alınmadı: %v", source.Name, err)
			continue
		}

		if len(items) == 0 {
			continue
		}

		// Qovluq adı — boşluqları alt xətt ilə əvəz et
		dirName := strings.ToLower(strings.ReplaceAll(source.Name, " ", "_"))
		dirPath := fmt.Sprintf("exports/%s", dirName)

		if err := os.MkdirAll(dirPath, 0755); err != nil {
			log.Printf("❌ Qovluq yaradılmadı [%s]: %v", dirPath, err)
			continue
		}

		// BUG FIX: əvvəlki versiya `export_{tarix}.json` yazırdı — bu, server
		// işləyərkən avtomatik exporter-in (internal/service/exporter) YAZDIĞI
		// EYNİ fayl idi. Server işləyərkən bu aləti əl ilə çağırsan, iki proses
		// eyni fayla yaza bilərdi (kim sonuncu yazsa, digərini məhv edərdi).
		// İndi fərqli, "_full" prefiksli fayla yazır ki, heç vaxt toqquşmasın —
		// bu, tam (bütün tarixçəni əhatə edən) mənbə dump-ıdır, avtomatik
		// exporter-in artımlı (incremental) faylından ayrıdır.
		fileName := fmt.Sprintf("%s/export_full_%s.json", dirPath, date)

		// BUG FIX: əvvəlki versiya birbaşa hədəf fayla yazırdı (os.Create +
		// Encode) — yazma zamanı proses kəsilsə, fayl yarımçıq/korlanmış
		// vəziyyətdə qala bilərdi. İndi (exporter.go-dakı eyni pattern)
		// əvvəlcə müvəqqəti fayla yazılır, sonra atomik rename edilir.
		tmpFile := fileName + ".tmp"
		file, err := os.Create(tmpFile)
		if err != nil {
			log.Printf("❌ Müvəqqəti fayl yaradılmadı [%s]: %v", tmpFile, err)
			continue
		}

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)

		if err := encoder.Encode(items); err != nil {
			file.Close()
			os.Remove(tmpFile)
			log.Printf("❌ JSON yazılmadı [%s]: %v", fileName, err)
			continue
		}

		if err := file.Sync(); err != nil {
			file.Close()
			os.Remove(tmpFile)
			log.Printf("❌ Fayl disk-ə yazılmadı [%s]: %v", fileName, err)
			continue
		}
		if err := file.Close(); err != nil {
			os.Remove(tmpFile)
			log.Printf("❌ Fayl bağlanmadı [%s]: %v", fileName, err)
			continue
		}
		if err := os.Rename(tmpFile, fileName); err != nil {
			os.Remove(tmpFile)
			log.Printf("❌ Fayl əvəz edilmədi [%s]: %v", fileName, err)
			continue
		}

		fmt.Printf("✅ [%s] → %s (%d xəbər)\n", source.Name, fileName, len(items))
	}

	fmt.Println("\nExport tamamlandı ✅")
}
