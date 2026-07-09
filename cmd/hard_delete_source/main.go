package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"example.com/new-scraper/internal/config"
	"example.com/new-scraper/internal/platform/database"
)

// cmd/hard_delete_source — bir mənbəni VƏ ona aid BÜTÜN feed_items
// sətirlərini DB-dən HƏMİŞƏLİK silir (feed_items.source_id FK-də
// "ON DELETE CASCADE" olduğu üçün, sources sətri silinəndə feed_items
// avtomatik silinir — bax migrations/001_init.sql).
//
// TƏHLÜKƏSİZLİK QEYDİ (niyə bu, HTTP API-də DEYİL, ayrıca CLI aləti kimi
// yazılıb): API-də olan DELETE /api/v1/sources/{id} yalnız SOFT DELETE edir
// (is_active=false, sətir qalır — bax source_handler.Delete). Əgər hard
// delete də HTTP üzərindən (məs. ?force=true parametri ilə) ekspoze
// olunsaydı, API_KEY-in sızması (log-a düşmə, kodda təsadüfən qalma və s.)
// TARİXİ MƏLUMATIN GERİ DÖNMƏZ İTKİSİNƏ səbəb ola bilərdi. Bu aləti işə
// salmaq üçün hücumçu təkcə API_KEY-i deyil, SERVERƏ FİZİKİ/SSH GİRİŞİ əldə
// etməlidir — bu, dağıdıcı bir əməliyyat üçün əhəmiyyətli dərəcədə daha
// güclü bir sərhəddir.
//
// TƏHLÜKƏSİZLİK QEYDİ 2 (niyə "yes/no" yox, adı tam yazmaq tələb olunur):
// Sadə bir "Davam edək? (y/n)" sualı, tərtibatçının konsolda tez-tez basdığı
// Enter/y düyməsi ilə TƏSADÜFƏN təsdiqlənə bilər. Silinəcək mənbənin ADINI
// hərfi-hərfinə yazmaq tələbi (GitHub-un repo silmə axınına bənzər), bu
// riski demək olar sıfıra endirir — çünki adı DƏQİQ bilmək və şüurlu şəkildə
// yazmaq lazımdır.
//
// İşlətmək:
//
//	go run ./cmd/hard_delete_source --id=6
func main() {
	id := flag.Int64("id", 0, "silinəcək mənbənin ID-si (məcburidir)")
	flag.Parse()

	if *id <= 0 {
		fmt.Println("İstifadə: go run ./cmd/hard_delete_source --id=<mənbə ID-si>")
		os.Exit(1)
	}

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

	var name, feedURL string
	err = db.QueryRow(ctx, `SELECT name, feed_url FROM sources WHERE id = $1`, *id).Scan(&name, &feedURL)
	if err != nil {
		log.Fatal(fmt.Errorf("mənbə tapılmadı (id=%d): %w", *id, err))
	}

	var itemCount int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM feed_items WHERE source_id = $1`, *id).Scan(&itemCount); err != nil {
		log.Fatal(fmt.Errorf("feed_items sayı alınmadı: %w", err))
	}

	fmt.Printf("\n⚠️  DİQQƏT — HƏMİŞƏLİK SİLMƏ ƏMƏLİYYATI\n\n")
	fmt.Printf("  Mənbə:        %s (id=%d)\n", name, *id)
	fmt.Printf("  Feed URL:     %s\n", feedURL)
	fmt.Printf("  Bağlı xəbər:  %d ədəd (BUNLAR DA silinəcək — CASCADE)\n\n", itemCount)
	fmt.Printf("Bu əməliyyat GERİ QAYTARILA BİLMƏZ. Davam etmək üçün mənbənin adını\n")
	fmt.Printf("HƏRFİ-HƏRFİNƏ yazın (\"%s\"): ", name)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimRight(input, "\r\n")

	if input != name {
		fmt.Println("\nAd uyğun gəlmədi — əməliyyat LƏĞV EDİLDİ, heç nə silinmədi.")
		os.Exit(1)
	}

	tag, err := db.Exec(ctx, `DELETE FROM sources WHERE id = $1`, *id)
	if err != nil {
		log.Fatal(fmt.Errorf("silinmə uğursuz oldu: %w", err))
	}
	if tag.RowsAffected() == 0 {
		fmt.Println("Heç bir sətir silinmədi (mənbə artıq mövcud deyildi?).")
		return
	}

	fmt.Printf("\n✅ Mənbə (id=%d, %s) və ona aid %d xəbər həmişəlik silindi.\n", *id, name, itemCount)
}
