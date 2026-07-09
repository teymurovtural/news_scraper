package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"example.com/new-scraper/internal/config"
	"example.com/new-scraper/internal/domain"
	"example.com/new-scraper/internal/platform/database"
	"example.com/new-scraper/internal/repository"
	"example.com/new-scraper/internal/service/discovery"

	"gopkg.in/yaml.v3"
)

type SourceConfig struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	FeedURL  string `yaml:"feed_url"`
	Category string `yaml:"category"`
}

type SourcesFile struct {
	Sources []SourceConfig `yaml:"sources"`
}

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

	data, err := os.ReadFile("sources.yaml")
	if err != nil {
		log.Fatal(fmt.Errorf("sources.yaml oxunmadı: %w", err))
	}

	var sourcesFile SourcesFile
	if err := yaml.Unmarshal(data, &sourcesFile); err != nil {
		log.Fatal(fmt.Errorf("sources.yaml parse edilmədi: %w", err))
	}

	sourceRepo := repository.NewSourceRepository(db)
	discoverySvc := discovery.NewDiscoveryService()

	ctx := context.Background()

	added := 0
	skipped := 0
	failed := 0

	for _, s := range sourcesFile.Sources {
		feedURL := s.FeedURL

		if feedURL == "" {
			log.Printf("🔍 [%s] RSS axtarılır...", s.Name)
			var err error
			feedURL, err = discoverySvc.Discover(ctx, s.URL)
			if err != nil {
				log.Printf("❌ [%s] RSS tapılmadı: %v", s.Name, err)
				failed++
				continue
			}
			log.Printf("✅ [%s] RSS tapıldı → %s", s.Name, feedURL)
		} else {
			log.Printf("✅ [%s] Manuel RSS → %s", s.Name, feedURL)
		}

		source := &domain.Source{
			Name:         s.Name,
			SiteURL:      s.URL,
			FeedURL:      feedURL,
			Category:     s.Category,
			PollInterval: 900,
		}

		if err := sourceRepo.Create(ctx, source); err != nil {
			if errors.Is(err, domain.ErrDuplicateSource) {
				log.Printf("⏭️  [%s] artıq mövcuddur, keçilir", s.Name)
				skipped++
				continue
			}
			log.Printf("❌ [%s] xəta: %v", s.Name, err)
			failed++
			continue
		}

		log.Printf("➕ [%s] DB-yə əlavə edildi", s.Name)
		added++
	}

	fmt.Printf("\nTamamlandı — ✅ %d əlavə edildi, ⏭️  %d keçildi, ❌ %d uğursuz\n",
		added, skipped, failed)
}
