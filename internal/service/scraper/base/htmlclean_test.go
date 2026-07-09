package base

import (
	"strings"
	"testing"
)

// TestCleanArticleHTML_RemovesSpecifiedSelectors — removeSelectors-da
// göstərilən elementlərin silindiyini yoxlayır (məs. BleepingComputer-də
// "h1" silmə fix-i məhz bu mexanizmə əsaslanır).
func TestCleanArticleHTML_RemovesSpecifiedSelectors(t *testing.T) {
	raw := `<h1>Başlıq</h1><p>Əsl məzmun</p><div class="ad">Reklam</div>`

	result, err := CleanArticleHTML(raw, []string{"h1", "div.ad"})
	if err != nil {
		t.Fatalf("gözlənilməz xəta: %v", err)
	}

	if strings.Contains(result, "<h1") {
		t.Errorf("h1 silinməli idi, amma qalıb: %s", result)
	}
	if strings.Contains(result, "Reklam") {
		t.Errorf("div.ad silinməli idi, amma qalıb: %s", result)
	}
	if !strings.Contains(result, "Əsl məzmun") {
		t.Errorf("əsl məzmun səhvən silinib: %s", result)
	}
}

// TestCleanArticleHTML_StripsXSS — bluemonday sanitizasiyasının işlədiyini
// yoxlayır: onerror, javascript:, <script> kimi XSS vektorları silinməlidir,
// normal formatlaşdırma tag-ları (p, img, a) isə qalmalıdır.
func TestCleanArticleHTML_StripsXSS(t *testing.T) {
	raw := `<p onclick="alert(1)">salam</p><script>alert(2)</script><img src="x" onerror="alert(3)"><a href="javascript:alert(4)">link</a>`

	result, err := CleanArticleHTML(raw, nil)
	if err != nil {
		t.Fatalf("gözlənilməz xəta: %v", err)
	}

	dangerous := []string{"onclick", "onerror", "<script", "javascript:", "alert("}
	for _, d := range dangerous {
		if strings.Contains(result, d) {
			t.Errorf("TƏHLÜKƏLİ MƏZMUN QALIB: %q nəticədə tapıldı: %s", d, result)
		}
	}

	// Normal tag-lar qalmalıdır
	if !strings.Contains(result, "<p>salam</p>") {
		t.Errorf("normal <p> tag-ı gözlənilməz şəkildə dəyişib: %s", result)
	}
	if !strings.Contains(result, "<img") {
		t.Errorf("img tag-ı silinməməli idi: %s", result)
	}
}

// TestCleanArticleHTML_PreservesYouTubeIframe — bu, "video öz yerində
// qalmalıdır" tələbinin regressiya testidir: əvvəllər bluemonday BÜTÜN
// iframe-ləri silirdi (YouTube daxil), video ayrıca VideoURL sahəsinə
// çıxarılıb /view-də sabit yerə yapışdırılırdı — bu, videonun məqalədəki
// əsl sırasını pozurdu. İndi youtube.com/embed iframe-ləri content_html-in
// öz DOM mövqeyində qalmalıdır.
func TestCleanArticleHTML_PreservesYouTubeIframe(t *testing.T) {
	raw := `<p>Əvvəlki paraqraf</p><iframe src="//www.youtube.com/embed/x5-NDM91Q7E" allowfullscreen></iframe><p>Sonrakı paraqraf</p>`

	result, err := CleanArticleHTML(raw, nil)
	if err != nil {
		t.Fatalf("gözlənilməz xəta: %v", err)
	}

	if !strings.Contains(result, `<iframe src="//www.youtube.com/embed/x5-NDM91Q7E"`) {
		t.Errorf("YouTube iframe qorunmalı idi, amma yoxdur: %s", result)
	}

	// Sıra qorunmalıdır: "Əvvəlki" iframe-dən ƏVVƏL, "Sonrakı" iframe-dən SONRA gəlməlidir.
	beforeIdx := strings.Index(result, "Əvvəlki")
	iframeIdx := strings.Index(result, "<iframe")
	afterIdx := strings.Index(result, "Sonrakı")
	if !(beforeIdx < iframeIdx && iframeIdx < afterIdx) {
		t.Errorf("video öz DOM sırasında deyil: %s", result)
	}
}

// TestCleanArticleHTML_RemovesNonYouTubeIframe — YouTube-dan başqa domendən
// gələn iframe-lər (reklam, tracker və s.) hələ də silinməlidir — ağ siyahı
// yalnız youtube.com/embed-ə aiddir, hər iframe-ə deyil.
func TestCleanArticleHTML_RemovesNonYouTubeIframe(t *testing.T) {
	raw := `<p>mətn</p><iframe src="https://evil-tracker.com/embed/x"></iframe>`

	result, err := CleanArticleHTML(raw, nil)
	if err != nil {
		t.Fatalf("gözlənilməz xəta: %v", err)
	}

	if strings.Contains(result, "<iframe") {
		t.Errorf("YouTube olmayan iframe silinməli idi, amma qalıb: %s", result)
	}
}

// TestCleanArticleHTML_RemovesScriptStyleByDefault — heç bir removeSelectors
// verilməsə belə, script/style/noscript həmişə silinməlidir.
func TestCleanArticleHTML_RemovesScriptStyleByDefault(t *testing.T) {
	raw := `<p>mətn</p><script>kod</script><style>css</style><noscript>fallback</noscript>`

	result, err := CleanArticleHTML(raw, nil)
	if err != nil {
		t.Fatalf("gözlənilməz xəta: %v", err)
	}

	for _, tag := range []string{"<script", "<style", "<noscript"} {
		if strings.Contains(result, tag) {
			t.Errorf("%s silinməli idi: %s", tag, result)
		}
	}
}
