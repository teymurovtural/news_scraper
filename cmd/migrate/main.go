package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"example.com/new-scraper/internal/config"
	"example.com/new-scraper/internal/platform/database"
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

	ctx := context.Background()

	// FEATURE: hansı migration-ların artıq icra olunduğunu izləyən cədvəl.
	// Bundan əvvəl bu tool hər çağırışda BÜTÜN .sql fayllarını yenidən icra
	// edirdi — indiyə qədər hər fayl idempotent (IF NOT EXISTS və s.) yazıldığı
	// üçün problem yaratmırdı. Amma gələcəkdə idempotent olmayan bir migration
	// (məs. ALTER TABLE ... RENAME COLUMN) yazılsa, hər təkrar çağırışda xəta
	// verəcəkdi. İndi hər fayl yalnız BİR DƏFƏ icra olunur və qeydə alınır.
	//
	// Geriyə uyğunluq: mövcud DB-lərdə bu cədvəl hələ boş olacaq, ona görə ilk
	// çağırışda bütün fayllar "icra olunmamış" sayılıb yenidən işə düşəcək —
	// bu, təhlükəsizdir, çünki hamısı idempotentdir (IF NOT EXISTS və s.).
	// Bundan sonra artıq düzgün izlənəcək.
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		log.Fatal(fmt.Errorf("schema_migrations cədvəli yaradılmadı: %w", err))
	}

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(files)

	for _, path := range files {
		filename := filepath.Base(path)

		var alreadyApplied bool
		if err := db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`,
			filename,
		).Scan(&alreadyApplied); err != nil {
			log.Fatal(fmt.Errorf("migration statusu yoxlanmadı [%s]: %w", filename, err))
		}

		if alreadyApplied {
			fmt.Printf("⏭️  %s (artıq icra olunub)\n", filename)
			continue
		}

		sql, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(fmt.Errorf("migration oxunmadı [%s]: %w", path, err))
		}

		// Migration-un özü VƏ schema_migrations-a qeyd — bir tranzaksiyada.
		// Beləliklə, əgər migration SQL-i xəta versə, qeyd də əlavə olunmur
		// (əks halda "icra olundu" yazılıb özü icra olunmamış qala bilərdi).
		tx, err := db.Begin(ctx)
		if err != nil {
			log.Fatal(fmt.Errorf("tranzaksiya başladılmadı [%s]: %w", filename, err))
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			log.Fatal(fmt.Errorf("migration icra edilmədi [%s]: %w", filename, err))
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`,
			filename,
		); err != nil {
			tx.Rollback(ctx)
			log.Fatal(fmt.Errorf("migration qeydə alınmadı [%s]: %w", filename, err))
		}

		if err := tx.Commit(ctx); err != nil {
			log.Fatal(fmt.Errorf("tranzaksiya təsdiqlənmədi [%s]: %w", filename, err))
		}

		fmt.Printf("✅ %s\n", filename)
	}

	fmt.Println("Bütün migrationlar tamamlandı ✅")
}
