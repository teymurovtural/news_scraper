package handler

import (
	"fmt"
	"net"
	"net/url"
	"regexp"

	"example.com/new-scraper/internal/platform/netguard"
)

// validatePublicHTTPURL — verilmiş URL-in http/https sxemli, düzgün host-lu
// və DAXİLİ/PRIVATE şəbəkə ünvanına yönəlmədiyini yoxlayır.
//
// TƏHLÜKƏSİZLİK QEYDİ (SSRF): source_handler.Create-ə göndərilən feed_url və
// site_url dəyərləri sadəcə DB-də saxlanmır — sonradan SERVERİN ÖZÜ
// tərəfindən fetch olunur (fetcher.go-da gofeed RSS parse, scraper-lərdə
// Playwright page.Goto). Bu yoxlama olmasa, kimsə feed_url kimi serverin
// daxili şəbəkəsinə yönələn bir ünvan (localhost, DB portu, bulud metadata
// ünvanı 169.254.169.254 və s.) verə bilər və server bunu "RSS linki" adı
// ilə öz adından fetch edərdi — bu, klassik Server-Side Request Forgery
// (SSRF) zəifliyidir.
func validatePublicHTTPURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL parse edilmədi: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL sxemi http və ya https olmalıdır")
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL-də host yoxdur")
	}

	// Host birbaşa IP kimi yazılıbsa (məs. "http://127.0.0.1:5434"),
	// birbaşa yoxla — DNS lookup-a ehtiyac yoxdur.
	if ip := net.ParseIP(host); ip != nil {
		if netguard.IsDisallowedIP(ip) {
			return fmt.Errorf("daxili/private şəbəkə ünvanlarına icazə verilmir: %s", host)
		}
		return nil
	}

	// Domain adıdırsa, DNS-in hansı IP-lərə resolve etdiyini yoxla — domain
	// zahirən "public" görünsə belə, DNS cavabı private IP-yə yönəldilə bilər
	// ("DNS rebinding" adlanan texnika). Ona görə host adının özünə deyil,
	// faktiki resolve olunan IP-lərə baxırıq.
	//
	// QEYD: bu, yalnız YARADILMA ANINDA edilən bir yoxlamadır. Faktiki RSS
	// fetch-i (hər 15 dəqiqədən bir, fetcher.go tərəfindən) AYRICA, öz DNS
	// lookup-unu edir — DNS rebinding-in tam qarşısını almaq üçün bu, kifayət
	// deyil. Fetch-anı qorunması üçün bax: internal/platform/netguard
	// (SafeDialContext) və fetcher.go.
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("host resolve edilmədi: %w", err)
	}
	for _, ip := range ips {
		if netguard.IsDisallowedIP(ip) {
			return fmt.Errorf("host daxili/private şəbəkə ünvanına yönəlir: %s", host)
		}
	}

	return nil
}

// (isDisallowedIP artıq bu faylda deyil — internal/platform/netguard
// paketinə köçürülüb, çünki eyni məntiq fetcher.go tərəfindən də (fetch-anı
// SSRF yoxlaması üçün) istifadə olunur. Tək mənbə, iki istifadəçi.)

// validSourceName — yalnız hərf (Azərbaycan/Latın simvolları daxil deyil,
// sadəlik üçün ASCII saxlanılır), rəqəm, boşluq, tire və nöqtəyə icazə verir.
var validSourceName = regexp.MustCompile(`^[a-zA-Z0-9 .\-]{2,100}$`)

// validateSourceName — mənbə adının (source.Name) təhlükəsiz olduğunu
// yoxlayır.
//
// TƏHLÜKƏSİZLİK QEYDİ (Path Traversal): source.Name sadəcə DB-də
// saxlanmır — exporter.go bunu birbaşa fayl sistemi qovluq adı qurmaq
// üçün istifadə edir (bax exporter.go: `exports/<name-with-spaces-replaced>`).
// Validasiya olmasa, "../../etc/cron.d/evil" kimi bir ad "exports/"
// qovluğundan kənara çıxıb serverin fayl sistemində istənilən yerə yazmaq
// üçün istifadə oluna bilərdi (klassik path traversal zəifliyi). Yalnız
// hərf/rəqəm/boşluq/tire/nöqtəyə icazə verməklə, "/", "\" və ".." kimi
// yol-idarəetmə simvollarının Name-ə düşməsinin qarşısı tamamilə alınır.
func validateSourceName(name string) error {
	if !validSourceName.MatchString(name) {
		return fmt.Errorf("yalnız hərf, rəqəm, boşluq, tire və nöqtəyə icazə verilir (2-100 simvol)")
	}
	return nil
}
