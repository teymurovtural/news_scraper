package generic

import (
	"log/slog"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Hook — YAML-dan referans olunan, kiçik və YENİDƏN İSTİFADƏ OLUNAN bir
// səhifə əməliyyatı. Məqsəd: popup bağlama, scroll-trigger kimi sayta-xas
// DAVRANIŞLARI da (yalnız selector-ları yox) Go kodu yazmadan, YAML-da təsvir
// edə bilmək. "eval_js" tipi bunun "escape hatch"-idir — hər hansı çox nadir,
// başqa heç bir tipə uyğun gəlməyən DOM manipulyasiyası belə, yeni Go kodu
// tələb etmədən, birbaşa YAML-a bir JS sətri kimi yazıla bilər.
type Hook struct {
	Type string `yaml:"type"`

	Selector string `yaml:"selector,omitempty"` // wait_attached, click_if_visible
	CSS      string `yaml:"css,omitempty"`      // inject_css
	JS       string `yaml:"js,omitempty"`       // eval_js
	Millis   int    `yaml:"millis,omitempty"`   // sleep; scroll_bottom_top-da son gözləmə; click_if_visible-da görünmə gözləməsi
	Repeat   int    `yaml:"repeat,omitempty"`   // eval_js: neçə dəfə icra edilsin (default 1)
}

// runHook — bir Hook-u icra edir. Bütün xətalar sadəcə log-lanır (Warn) —
// bir hook-un uğursuz olması bütün scrape-i uğursuz etməməlidir, çünki
// bunlar əksər halda "best-effort" təkmilləşdirmələrdir (popup bağlanmasa
// da, çox güman content hələ də oxuna biləcək).
func runHook(page playwright.Page, h Hook, actionTimeout float64, sourceName, link string) {
	switch h.Type {
	case "wait_attached":
		// BUG FIX: .First() olmadan, Selector birdən çox elementə uyğun
		// gələndə Playwright-in "strict mode" davranışı xəta verir (məs.
		// securityweek-in "div.zox-post-body p" selector-u onlarla <p>-ə
		// uyğun gəlir). Orijinal sayta-xas scraper-lər bunun üçün həmişə
		// .First() çağırırdı — bu hook da eyni şəkildə YALNIZ birinci
		// uyğunluğu gözləməlidir (məqsəd "konteyner DOM-a düşdümü"
		// yoxlamaqdır, neçə element olduğunu yox).
		if err := page.Locator(h.Selector).First().WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateAttached,
			Timeout: playwright.Float(actionTimeout),
		}); err != nil {
			slog.Warn("generic: wait_attached uğursuz", "source", sourceName, "link", link, "selector", h.Selector, "error", err)
		}

	case "scroll_bottom_top":
		// Bir çox sayt lazy-load content/reklamı yalnız scroll ediləndə
		// DOM-a əlavə edir. Aşağı-yuxarı scroll bunu trigger edir, sonra
		// səhifəni əvvəlki vəziyyətinə (yuxarı) qaytarır.
		if _, err := page.Evaluate(`() => window.scrollTo(0, document.body.scrollHeight)`); err != nil {
			slog.Warn("generic: scroll (aşağı) xətası", "source", sourceName, "link", link, "error", err)
		}
		time.Sleep(1 * time.Second)
		if _, err := page.Evaluate(`() => window.scrollTo(0, 0)`); err != nil {
			slog.Warn("generic: scroll (yuxarı) xətası", "source", sourceName, "link", link, "error", err)
		}
		wait := 500
		if h.Millis > 0 {
			wait = h.Millis
		}
		time.Sleep(time.Duration(wait) * time.Millisecond)

	case "inject_css":
		if _, err := page.AddStyleTag(playwright.PageAddStyleTagOptions{
			Content: playwright.String(h.CSS),
		}); err != nil {
			slog.Warn("generic: CSS inject edilmədi", "source", sourceName, "link", link, "error", err)
		}

	case "eval_js":
		repeat := h.Repeat
		if repeat < 1 {
			repeat = 1
		}
		for i := 0; i < repeat; i++ {
			if _, err := page.Evaluate(h.JS); err != nil {
				slog.Warn("generic: JS icra edilmədi", "source", sourceName, "link", link, "error", err)
			}
		}

	case "click_if_visible":
		loc := page.Locator(h.Selector)
		timeout := actionTimeout
		if h.Millis > 0 {
			timeout = float64(h.Millis)
		}
		if err := loc.WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(timeout),
		}); err == nil {
			if err := loc.Click(); err != nil {
				slog.Warn("generic: klik uğursuz", "source", sourceName, "link", link, "selector", h.Selector, "error", err)
			} else {
				slog.Info("generic: elementə klikləndi", "source", sourceName, "link", link, "selector", h.Selector)
			}
		}
		// Görünmədisə (WaitFor timeout) — bu, GÖZLƏNİLƏN haldır (popup hər
		// zaman çıxmır), ona görə xəta loglanmır.

	case "sleep":
		time.Sleep(time.Duration(h.Millis) * time.Millisecond)

	default:
		slog.Warn("generic: naməlum hook tipi", "source", sourceName, "type", h.Type)
	}
}
