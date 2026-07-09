package base

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
)

// youtubeEmbedSrc — yalnız youtube.com/-nocookie.com/embed/ ünvanlarına
// uyğun gələn iframe src-lərinə icazə verir (http:, https:, ya da
// protokol-nisbi "//..." formatında). Başqa domendən olan iframe-lər
// (məs. reklam/tracker) hələ də silinir.
var youtubeEmbedSrc = regexp.MustCompile(`^(https?:)?//(www\.)?youtube(-nocookie)?\.com/embed/`)

// sanitizePolicy — bütün scraper-lər bu paylaşılan policy-ni istifadə edir.
// UGCPolicy() məqalə formatlaşdırması üçün lazım olan tag-ları saxlayır
// (p, h1-h6, img, a[href], blockquote, figure, figcaption, pre, code, ul/ol/li
// və s.), amma XSS vektoru ola biləcək hər şeyi (on* atributlar, javascript:
// sxemi, <script> və s.) təmizləyir.
//
// BUG FIX: əvvəllər BÜTÜN <iframe>-lər silinirdi (UGCPolicy defolt olaraq
// iframe-ə icazə vermir). Nəticədə YouTube video embed-ləri content_html-dən
// itir, VideoURL adlı ayrıca sahədə saxlanılıb /view şablonunda SABİT bir
// yerə (həmişə məqalənin əvvəlinə) yapışdırılırdı — bu, videonun məqalədə
// əsl göründüyü sırasını pozurdu. İndi YALNIZ youtube.com/embed domenindən
// olan iframe-lərə icazə veririk — video öz DOM mövqeyində (content_html-in
// daxilində, hardasa idisə orda) qalır, sıra pozulmur. Başqa domenlərdən olan
// iframe-lər (reklam, tracker və s.) hələ də silinir.
var sanitizePolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements("iframe")
	p.AllowAttrs("src").Matching(youtubeEmbedSrc).OnElements("iframe")
	p.AllowAttrs("width", "height", "frameborder", "allow", "allowfullscreen").OnElements("iframe")
	return p
}()

// CleanArticleHTML — verilmiş raw HTML fraqmentini (məqalə konteynerinin
// innerHTML-i) təmizləyir: script/style teqlərini və `removeSelectors`-da
// göstərilən "artıq" bloqları (reklam, share düymələri, related-links və s.)
// silir. Struktur (paraqraf sırası, başlıqlar, şəkillər, YouTube video-lar)
// toxunulmaz qalır.
//
// removeSelectors hər sayta xasdır — hər scraper öz "zibil" seçicilərini ötürür.
//
// SON ADDIM olaraq nəticə bluemonday ilə sanitize olunur — bu, mənbə saytın
// öz HTML-ində gizli qala bilən XSS-i (onerror, javascript: href və s.) tutur.
// Bu, /view endpoint-inin, JSON API cavablarının və export fayllarının hamısını
// eyni vaxtda qoruyur, çünki hamısı elə bu funksiyanın nəticəsini istifadə edir.
func CleanArticleHTML(rawHTML string, removeSelectors []string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return "", err
	}

	// Həmişə silinən, təhlükəsizlik/lazımsızlıq baxımından universal olanlar.
	// Diqqət: youtube.com/embed iframe-ləri bura daxil deyil (aşağıda
	// sanitizePolicy səviyyəsində ağ siyahıya salınıb, silinmir).
	doc.Find("script, style, noscript, iframe[src*='doubleclick'], iframe[src*='googlesyndication']").Remove()

	for _, sel := range removeSelectors {
		doc.Find(sel).Remove()
	}

	// goquery bütöv sənəd kimi parse edir (html/body əlavə edir),
	// ona görə əsl content-i body-nin içindən çıxarırıq.
	html, err := doc.Find("body").First().Html()
	if err != nil {
		return "", err
	}

	cleaned := strings.TrimSpace(html)
	return sanitizePolicy.Sanitize(cleaned), nil
}

// HTMLToPlainText — ContentHTML-dən keyword-matching üçün sadə mətn çıxarır.
// Teqlər atılır, mətn qalır (paraqraflar arasında boşluq saxlanılır).
func HTMLToPlainText(rawHTML string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return "", err
	}
	doc.Find("script, style, noscript").Remove()
	text := doc.Find("body").First().Text()
	return strings.TrimSpace(text), nil
}
