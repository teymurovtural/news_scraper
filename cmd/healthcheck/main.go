package main

import (
	"context"
	"fmt"
	"log"

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

	var result int
	err = db.QueryRow(context.Background(), "SELECT 1").Scan(&result)
	if err != nil {
		log.Fatal(fmt.Errorf("DB bağlantısı uğursuz: %w", err))
	}

	fmt.Println("✅ DB bağlantısı uğurludur")
	fmt.Printf("   Host: %s\n", cfg.DB.Host)
	fmt.Printf("   Port: %s\n", cfg.DB.Port)
	fmt.Printf("   DB:   %s\n", cfg.DB.Name)
	fmt.Printf("   User: %s\n", cfg.DB.User)
}
