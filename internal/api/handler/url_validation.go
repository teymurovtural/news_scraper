package handler

import (
	"fmt"
	"net"
	"net/url"
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
		if isDisallowedIP(ip) {
			return fmt.Errorf("daxili/private şəbəkə ünvanlarına icazə verilmir: %s", host)
		}
		return nil
	}

	// Domain adıdırsa, DNS-in hansı IP-lərə resolve etdiyini yoxla — domain
	// zahirən "public" görünsə belə, DNS cavabı private IP-yə yönəldilə bilər
	// ("DNS rebinding" adlanan texnika). Ona görə host adının özünə deyil,
	// faktiki resolve olunan IP-lərə baxırıq.
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("host resolve edilmədi: %w", err)
	}
	for _, ip := range ips {
		if isDisallowedIP(ip) {
			return fmt.Errorf("host daxili/private şəbəkə ünvanına yönəlir: %s", host)
		}
	}

	return nil
}

// isDisallowedIP — loopback (127.0.0.1, ::1), private (RFC1918: 10.x, 172.16-31.x,
// 192.168.x), link-local (169.254.x.x — bulud metadata ünvanları da buradadır)
// və s. "daxili" sayılan IP-ləri tanıyır.
func isDisallowedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}
