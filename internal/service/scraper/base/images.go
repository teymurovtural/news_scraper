package base

import (
	"net/url"
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

// ResolveURL — nisbi (relative) bir URL-i (məs. "/images/x.jpg" və ya
// protokol-nisbi "//cdn.example.com/x.jpg") səhifənin öz ünvanına (pageURL)
// görə mütləq (absolute) URL-ə çevirir. src artıq mütləqdirsə (http/https
// ilə başlayırsa) və ya "data:image" URI-dirsə, toxunulmadan qaytarılır.
//
// BUG FIX: bəzi saytlar (məs. CyberScoop-un cover şəkli) <img> atributunda
// mütləq deyil, nisbi yol qaytara bilir. Bu, DB-yə/export-a olduğu kimi
// "/foo.jpg" şəklində yazılırdısa, /view səhifəsində və ya JSON export-u
// istifadə edən istənilən xarici alətdə şəkil sınmış link kimi görünürdü
// (çünki nisbi yol o alətin ÖZ domenindən axtarılır, mənbə saytdan yox).
func ResolveURL(pageURL, src string) string {
	if src == "" || strings.HasPrefix(src, "data:image") {
		return src
	}

	baseURL, err := url.Parse(pageURL)
	if err != nil {
		return src
	}
	ref, err := url.Parse(src)
	if err != nil {
		return src
	}
	return baseURL.ResolveReference(ref).String()
}
