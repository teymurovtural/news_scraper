package base

import (
	"strings"

	"github.com/playwright-community/playwright-go"
)

// ExtractLazyImageAttr — bir <img> Locator-undan həqiqi URL-i çıxarır.
// Bir çox sayt lazy-load işlədir: src-də 1px placeholder (və ya boş) olur,
// əsl URL isə data-src atributundadır. Bu funksiya əvvəlcə data-src-i yoxlayır,
// tapılmasa src-ə keçir, "data:image" (inline base64 placeholder) ilə
// başlayanları rədd edir.
//
// Bu pattern əvvəllər thehackernews, bleepingcomputer, itsecurityguru və
// darkreading scraper-lərində demək olar eyni şəkildə təkrarlanırdı
// (GoLand "Duplicated code fragment" xəbərdarlığı) — indi hamısı bu ortaq
// funksiyanı çağırır.
//
// Sayta xas əlavə filtrlər (məs. bleepingcomputer-in "yalnız bleepstatic.com
// domenindən" şərti) bu funksiyanın nəticəsi üzərində çağıran tərəfindən
// tətbiq olunmalıdır — bu funksiya yalnız ümumi lazy-load məntiqini həll edir.
//
// Uyğun URL tapılmasa (boş, ya da data: URI-dirsə), src="" qaytarır — çağıran
// bunu yoxlayıb item-i keçə bilər.
func ExtractLazyImageAttr(img playwright.Locator, timeoutMs float64) (src, alt string) {
	src, _ = img.GetAttribute("data-src", playwright.LocatorGetAttributeOptions{
		Timeout: playwright.Float(timeoutMs),
	})
	if src == "" {
		src, _ = img.GetAttribute("src", playwright.LocatorGetAttributeOptions{
			Timeout: playwright.Float(timeoutMs),
		})
	}
	if src == "" || strings.HasPrefix(src, "data:image") {
		return "", ""
	}

	alt, _ = img.GetAttribute("alt", playwright.LocatorGetAttributeOptions{
		Timeout: playwright.Float(timeoutMs),
	})
	return src, alt
}
