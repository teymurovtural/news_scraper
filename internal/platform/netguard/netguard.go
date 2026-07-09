// Package netguard, tətbiqin SSRF-ə (Server-Side Request Forgery) qarşı
// mühafizə məntiqini bir yerdə saxlayır. Bunun İKİ istifadə yeri var:
//
//  1. YARADILMA ANI (creation-time) — api/handler/url_validation.go, POST
//     /api/v1/sources-da yeni feed_url/site_url daxil edilən anda yoxlayır.
//  2. FETCH ANI (fetch-time) — service/fetcher/fetcher.go, hər 15 dəqiqədən
//     bir HƏR mənbə üçün RSS çəkiləndə yoxlayır.
//
// TƏHLÜKƏSİZLİK QEYDİ (DNS Rebinding / TOCTOU): yalnız (1)-i etmək kifayət
// deyil — çünki DNS qeydi yaradılma anından SONRA dəyişə bilər (məs. çox qısa
// TTL ilə, əvvəlcə public IP-yə, sonra 169.254.169.254 kimi daxili bir
// ünvana yönəldilə bilər). Nəticədə "yoxlanan an" (creation-time) ilə
// "istifadə olunan an" (hər fetch) arasında bir boşluq yaranır (bu, "Time-
// Of-Check-to-Time-Of-Use", qısaca TOCTOU adlanır). SafeDialContext bu
// boşluğu bağlayır — DNS-i "yoxlama" ilə "faktiki qoşulma" arasında SIFIR
// fasilə ilə, EYNİ funksiya çağırışı daxilində edir.
package netguard

import (
	"context"
	"fmt"
	"net"
)

// IsDisallowedIP — loopback (127.0.0.1, ::1), private (RFC1918: 10.x,
// 172.16-31.x, 192.168.x), link-local (169.254.x.x — bulud metadata
// ünvanları da buradadır) və s. "daxili" sayılan IP-ləri tanıyır.
//
// Bu, api/handler/url_validation.go-dakı (yaradılma-anı yoxlaması) və bu
// paketdəki SafeDialContext-in (fetch-anı yoxlaması) ORTAQ TƏK mənbəyidir —
// "daxili IP" nə deməkdir sualının cavabı YALNIZ burda təyin olunur, iki
// yerdə təkrarlanmır.
func IsDisallowedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

// SafeDialContext — http.Transport.DialContext kimi istifadə olunmaq üçün
// nəzərdə tutulub. Adi http.Transport-un default DialContext-i sadəcə
// hostname-i verilmiş ünvana qoşulur — bu, Go-nun öz DNS lookup-unu edir,
// bizim əvvəlcədən (yaratma anında) etdiyimiz yoxlamadan tamamilə xəbərsiz.
//
// Bu funksiya bunun əvəzinə:
//  1. Host-u DNS-dən özü resolve edir (bu, faktiki bağlantı ANINDA olur,
//     scheduler-in hər 15 dəqiqədən bir çağırdığı FetchSource-un DAXİLİNDƏ)
//  2. Qayıdan İLK IP-nin daxili/private olmadığını yoxlayır
//  3. Həmin YOXLANMIŞ IP-yə BİRBAŞA qoşulur (hostname-ə yenidən müraciət
//     ETMİR) — addım 2 ilə addım 3 arasında YENİ bir DNS lookup olmadığı
//     üçün, bu iki addım arasında domenin sahibinin DNS-i dəyişdirməsi
//     (rebinding) mümkün deyil, çünki artıq IP-nin özü ilə işləyirik.
func SafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("netguard: ünvan ayrılmadı (%s): %w", addr, err)
	}

	// Host artıq IP kimi yazılıbsa (nadir haldır, adətən DNS adı gəlir),
	// DNS-ə ehtiyac yoxdur, birbaşa yoxla.
	if ip := net.ParseIP(host); ip != nil {
		if IsDisallowedIP(ip) {
			return nil, fmt.Errorf("netguard: daxili/private IP-yə qoşulma rədd edildi: %s", ip)
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("netguard: host resolve edilmədi (%s): %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("netguard: host üçün heç bir IP tapılmadı: %s", host)
	}

	for _, ipAddr := range ips {
		if IsDisallowedIP(ipAddr.IP) {
			return nil, fmt.Errorf("netguard: host daxili/private IP-yə yönəlir: %s → %s", host, ipAddr.IP)
		}
	}

	// BİLƏRƏKDƏN hostname-ə YOX, artıq yoxlanmış İLK IP-yə birbaşa
	// qoşuluruq — TOCTOU pəncərəsini bağlayan addım məhz budur.
	dialAddr := net.JoinHostPort(ips[0].IP.String(), port)
	return (&net.Dialer{}).DialContext(ctx, network, dialAddr)
}
