package base

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"time"

	"example.com/new-scraper/internal/platform/netguard"

	"github.com/playwright-community/playwright-go"
)

// hostGuard — bir browserCtx (bir scrape chunk-ı) ömrü boyunca yaşayan,
// host → (icazəlidir/deyil, səbəb) keşi.
//
// TƏHLÜKƏSİZLİK QEYDİ (Playwright SSRF boşluğu): internal/platform/netguard
// (SafeDialContext) YALNIZ Go-nun öz http.Client-ini (gofeed-in RSS
// fetch-i) qoruyur. Playwright/Chromium tamamilə öz şəbəkə stack-i ilə
// işləyir — page.Goto(fi.Link) çağırışı əvvəllər HEÇ bir IP yoxlamasından
// keçmirdi. getScraperForLink linki yalnız scraper_configs.yaml-da
// müəyyən olunmuş sabit domenlərlə (prefix) məhdudlaşdırır, ona görə
// praktik risk aşağıdır, amma sıfır deyil: həmin domenlərdən BİRİNİN
// DNS-i kompromis olsa (DNS rebinding/cache poisoning), Chromium heç bir
// yoxlama olmadan birbaşa qoşulardı. hostGuard bu boşluğu bağlayır.
//
// KEŞ NİYƏ LAZIMDIR: browserCtx.Route hər tab-dakı HƏR sorğunu (naviqasiya,
// şəkil, JS, CSS, XHR) tutur — bir chunk-da 5 tab, hər biri onlarla sorğu
// göndərə bilər. Keş olmasa, hər sorğu üçün ayrı net.LookupIP çağırmaq
// lazım gələrdi. Amma bir browserCtx ömrü (bir chunk-ın scrape müddəti,
// adətən saniyələr) ərzində eyni host üçün DNS nəticəsinin dəyişməsi
// praktik olaraq əhəmiyyətsizdir — ona görə hər unikal host YALNIZ BİR
// DƏFƏ resolve olunur, sonrakı bütün sorğular üçün keşdən oxunur.
type hostGuard struct {
	mu    sync.Mutex
	cache map[string]guardResult
}

type guardResult struct {
	allowed bool
	reason  string // yalnız allowed=false olanda mənalıdır, loq üçün
}

func newHostGuard() *hostGuard {
	return &hostGuard{cache: make(map[string]guardResult)}
}

// allowed — host-un icazəli olub-olmadığını (və rədd səbəbini) qaytarır.
// Nəticə keşlənir.
func (g *hostGuard) allowed(host string) guardResult {
	g.mu.Lock()
	if v, ok := g.cache[host]; ok {
		g.mu.Unlock()
		return v
	}
	g.mu.Unlock()

	res := resolveAndCheck(host)

	g.mu.Lock()
	g.cache[host] = res
	g.mu.Unlock()

	return res
}

// resolveAndCheck — host-u DNS-dən (Go-nun öz resolver-i ilə, Chromium-un
// DNS-indən MÜSTƏQİL) resolve edir və bütün qayıdan IP-lərin
// daxili/private olmadığını yoxlayır. Bax: netguard.IsDisallowedIP (eyni
// "daxili IP" tərifi — tək mənbə, üç istifadəçi: yaradılma-anı,
// RSS-fetch-anı, indi də Playwright-naviqasiya-anı).
func resolveAndCheck(host string) guardResult {
	if ip := net.ParseIP(host); ip != nil {
		if netguard.IsDisallowedIP(ip) {
			return guardResult{false, fmt.Sprintf("disallowed_ip: %s", ip)}
		}
		return guardResult{true, ""}
	}

	ips, err := lookupWithRetry(host)
	if err != nil {
		return guardResult{false, fmt.Sprintf("dns_error: %v", err)}
	}
	if len(ips) == 0 {
		return guardResult{false, "no_ip_returned"}
	}
	for _, ip := range ips {
		if netguard.IsDisallowedIP(ip) {
			return guardResult{false, fmt.Sprintf("disallowed_ip: %s", ip)}
		}
	}
	return guardResult{true, ""}
}

// lookupWithRetry — net.LookupIP-in "context-siz, retry-siz" versiyasına
// nisbətən: hər cəhdə 3 saniyəlik sərt timeout qoyur (asılı qalan DNS
// sorğusu bütün chunk-ı gecikdirməsin deyə) və İLK cəhd uğursuz olsa,
// 200ms gözləyib BİR DƏFƏ də cəhd edir (müvəqqəti şəbəkə/DNS yüklənməsinə
// tab vermək üçün).
func lookupWithRetry(host string) ([]net.IP, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		cancel()

		if err == nil && len(addrs) > 0 {
			ips := make([]net.IP, len(addrs))
			for i, a := range addrs {
				ips[i] = a.IP
			}
			return ips, nil
		}
		lastErr = err
		if attempt == 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	return nil, lastErr
}

// registerSSRFGuard — verilmiş browserCtx-də BÜTÜN şəbəkə sorğularını
// tutan bir route qoyur; daxili/private IP-yə yönələn hər sorğu (naviqasiya
// daxil) bağlanmadan (Chromium qoşulmadan) əvvəl rədd edilir.
//
// QEYD (qalan TOCTOU pəncərəsi): Go-dakı SafeDialContext-dən fərqli olaraq,
// burda "yoxlanmış IP-yə birbaşa qoşulma" mümkün deyil — Chromium özü
// növbəti addımda YENİDƏN öz DNS-ini işlədəcək. Yəni bizim yoxlama ilə
// Chromium-un faktiki qoşulması arasında çox kiçik bir pəncərə qalır. Bu,
// Playwright-un məhdudiyyətidir (öz Dialer-imizi ötürə bilmirik), sıfır
// yoxlamadan qat-qat yaxşıdır və 6 sabit, öncədən bilinən domen üçün real
// eksploatasiya çətinliyi yüksəkdir.
func registerSSRFGuard(browserCtx playwright.BrowserContext) error {
	guard := newHostGuard()

	return browserCtx.Route("**/*", func(route playwright.Route) {
		req := route.Request()

		u, err := url.Parse(req.URL())
		if err != nil {
			route.Abort("failed")
			return
		}

		host := u.Hostname()
		if host == "" {
			route.Continue()
			return
		}

		res := guard.allowed(host)
		if !res.allowed {
			slog.Warn("base: SSRF qorunması - sorğu bloklandı",
				"host", host, "url", req.URL(), "resource_type", req.ResourceType(), "reason", res.reason)
			route.Abort("blockedbyclient")
			return
		}

		route.Continue()
	})
}
