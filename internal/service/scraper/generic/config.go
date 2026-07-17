package generic

// Package generic — "config-driven" scraper. Məqsəd: yeni bir mənbə (sayt)
// əlavə etmək üçün YENİ Go kodu yazmaq YOX, yalnız bu strukturların uyğun
// gələn YAML təsvirini (bax scraper_configs.yaml) yazmaq kifayət etsin.
//
// DİZAYN QƏRARI: 6 mövcud scraper-i (thehackernews, securityweek,
// bleepingcomputer, cyberscoop, itsecurityguru, darkreading) müqayisə etdikdə
// aydın oldu ki, onların ~90%-i eyni PATTERN-dir (selector tap → mətn/HTML
// çıxar → təmizlə → şəkilləri tap → video axtar), yalnız KONKRET selector-lar
// fərqlidir. Qalan ~10% (popup bağlama, scroll-trigger, mətn normalizasiyası)
// isə həqiqətən sayta-xas DAVRANIŞDIR — bunun üçün "Hook" konsepti var (bax
// hooks.go): kiçik, adlandırılmış, YENİDƏN İSTİFADƏ OLUNAN əməliyyatlar
// (scroll, popup bağla, JS icra et), YAML-dan referans olunur. Beləliklə
// yeni "normal" sayt üçün Go kodu YOX, yalnız data lazımdır; yalnız HƏQİQƏTƏN
// yeni bir davranış growth (məs. tamam fərqli bir captcha-bypass) tələb
// olunanda yeni hook tipi (Go kodu) əlavə olunmalıdır.
type SourceConfig struct {
	// Prefix — bu konfiqurasiyanın hansı linklərə aid olduğunu təyin edir
	// (scraper_service.go-dakı getScraperForLink ilə eyni prefix-matching
	// məntiqi). Məs. "https://thehackernews.com".
	Prefix string `yaml:"prefix"`
	// Name — log mesajlarında və xəta izahlarında istifadə olunur.
	Name string `yaml:"name"`
	// ActionTimeoutMs — bu mənbənin bütün DOM əməliyyatları (InnerText,
	// InnerHTML, GetAttribute, WaitFor) üçün vahid timeout. Boş/0 olsa,
	// 5000ms default istifadə olunur (bax scraper.go).
	ActionTimeoutMs float64 `yaml:"action_timeout_ms,omitempty"`

	// PreHooks — səhifə yüklənəndən sonra, title/author/date çıxarılmazdan
	// ƏVVƏL işə düşür (popup bağlama, scroll, DOM-dan lazımsız blok silmə).
	PreHooks []Hook `yaml:"pre_hooks,omitempty"`
	// MidHooks — title/author/date/excerpt çıxarılandan sonra, content HTML
	// çıxarılmazdan ƏVVƏL işə düşür (lazy-load content-i trigger etmək üçün
	// scroll, dinamik reklam bloklarını silmək və s.).
	MidHooks []Hook `yaml:"mid_hooks,omitempty"`

	Title  FieldSelector `yaml:"title"`
	Author FieldSelector `yaml:"author"`
	Date   FieldSelector `yaml:"date"`
	// Excerpt — bəzi saytlarda title-dan sonra gələn qısa giriş paraqrafı,
	// varsa content HTML-in əvvəlinə əlavə olunur.
	Excerpt *ExcerptConfig `yaml:"excerpt,omitempty"`

	// ContentSelector — məqalənin əsas HTML konteynerinin selector-u.
	ContentSelector string `yaml:"content_selector"`
	// RemoveSelectors — ContentSelector daxilində olan, amma məqaləyə aid
	// OLMAYAN blokları (reklam, related-posts, paylaşma düymələri) təmizləmək
	// üçün (bax base.CleanArticleHTML).
	RemoveSelectors []string `yaml:"remove_selectors,omitempty"`
	// ResolveLazyImagesInContentHTML — true olsa, content HTML-in daxilindəki
	// bütün img[data-src] atributları src-ə köçürülür (goquery ilə,
	// InnerHTML çəkildikdən DƏRHAL sonra, DOM-dan kənar, sətir üzərində).
	// Bəzi saytlar (thehackernews, bleepingcomputer) lazy-load üçün bunu
	// tələb edir — DOM-dakı Locator-lar üçün ExtractLazyImageAttr kifayətdir,
	// amma InnerHTML XAM STRING kimi çəkildiyi üçün, o string-in daxilindəki
	// img teqləri də ayrıca düzəldilməlidir.
	ResolveLazyImagesInContentHTML bool `yaml:"resolve_lazy_images_in_content_html,omitempty"`

	FeaturedImage *FeaturedImageConfig `yaml:"featured_image,omitempty"`
	ContentImages *ContentImagesConfig `yaml:"content_images,omitempty"`
	Video         *VideoConfig         `yaml:"video,omitempty"`
}

// FieldSelector — title/author/date kimi tək-mətnli sahələri necə tapmağı
// təsvir edir. Bütün variasiyalar (scope+nth, meta fallback, trim, title-case)
// 6 mövcud scraper-də faktiki istifadə olunan pattern-lərdir.
type FieldSelector struct {
	// Selector — DOM-da mətni tapmaq üçün əsas CSS selector.
	Selector string `yaml:"selector,omitempty"`
	// Scope — verilibsə, Selector bu konteynerin DAXİLİNDƏ axtarılır
	// (məs. thehackernews-də həm author, həm date "div.postmeta"
	// daxilindəki eyni "span.author" selector-udur, yalnız Nth fərqlidir).
	Scope string `yaml:"scope,omitempty"`
	// Nth — neçənci uyğunluq götürülsün (0 = birinci, Playwright-da
	// Nth(0) == First() ilə eynidir).
	Nth int `yaml:"nth,omitempty"`

	// MetaFallback — DOM-dan mətn tapılmasa (və ya boşdursa), bu meta tag
	// selector-unun "content" atributu istifadə olunur. Məs.
	// "meta[name='author']" və ya "meta[property='article:published_time']".
	MetaFallback string `yaml:"meta_fallback,omitempty"`
	// MetaFirst — true olsa, sıra TƏRSİNƏ çevrilir: əvvəlcə MetaFallback
	// yoxlanılır, yalnız o boş qayıdarsa DOM Selector-a keçilir. Bəzi
	// saytlarda (itsecurityguru-nun tarixi kimi) meta tag DOM-dan daha
	// etibarlıdır (ISO format, DOM-un mətn formatına nisbətən).
	MetaFirst bool `yaml:"meta_first,omitempty"`

	// TrimLeftChars/TrimRightChars — strings.TrimLeft/TrimRight-ə ötürülən
	// cutset (məs. securityweek tarixindəki "| " prefiksini, ya da
	// darkreading author-undakı sondakı "," işarəsini silmək üçün).
	TrimLeftChars  string `yaml:"trim_left_chars,omitempty"`
	TrimRightChars string `yaml:"trim_right_chars,omitempty"`
	// TitleCase — true olsa, "TIM STARKS" → "Tim Starks" kimi normalize
	// edir (cyberscoop-un CSS text-transform:uppercase effektini aradan
	// qaldırmaq üçün istifadə etdiyi pattern).
	TitleCase bool `yaml:"title_case,omitempty"`
}

// ExcerptConfig — title-dan sonra gələn qısa giriş paraqrafı.
type ExcerptConfig struct {
	Selector string `yaml:"selector"`
	// IsHTML — true olsa, InnerHTML (artıq HTML fraqmenti, escape edilmir —
	// cyberscoop pattern-i) çəkilir; false olsa, InnerText (plain mətn,
	// HTML-ə yerləşdirməzdən əvvəl html.EscapeString ilə escape olunur —
	// securityweek pattern-i, çünki mətndəki "<"/">"/"&" simvolları
	// təsadüfən tag/entity kimi parse oluna bilər).
	IsHTML bool `yaml:"is_html,omitempty"`
}

// FeaturedImageConfig — məqalənin əsas/cover şəkli.
type FeaturedImageConfig struct {
	Selector string `yaml:"selector"`
	// ResolveURL — true olsa, nisbi (relative) URL-lər səhifənin öz
	// ünvanına görə mütləqə çevrilir (bax base.ResolveURL; cyberscoop-un
	// cover şəkli üçün lazımdır).
	ResolveURL bool `yaml:"resolve_url,omitempty"`
	// Prepend — true olsa, bu şəkil ContentSelector-un HTML-i DAXİLİNDƏ
	// DEYİL (fiziki olaraq kənarda yerləşir), ona görə əl ilə <img> tag-ı
	// kimi content HTML-in əvvəlinə əlavə olunur (cyberscoop, securityweek,
	// itsecurityguru, darkreading pattern-i). false olsa, şəkil artıq
	// ContentSelector-un içindədir (thehackernews-də div.separator
	// div#articlebody-nin bir hissəsidir) — yalnız images siyahısına
	// (dublikat yoxlanılaraq) əlavə olunur, HTML toxunulmur.
	Prepend bool `yaml:"prepend,omitempty"`
}

// ContentImagesConfig — məqalə mətni daxilindəki adi şəkillər.
type ContentImagesConfig struct {
	Selector string `yaml:"selector"`
	// DomainContains — verilibsə, URL-də bu qatarlardan ƏN AZI biri
	// olmalıdır (OR məntiqi). Boşdursa filtr tətbiq olunmur.
	DomainContains []string `yaml:"domain_contains,omitempty"`
	// PathContains — DomainContains kimi, amma ayrıca yol seqmenti şərti
	// üçün (bleepingcomputer-in "content/hl-images" VƏ YA "images/news"
	// tələbi kimi — hər iki filtr eyni vaxtda AND məntiqi ilə tətbiq olunur).
	PathContains []string `yaml:"path_contains,omitempty"`
	// ExcludeContains — URL-in (kiçik hərflə) bu qatarlardan HEÇ BİRİNİ
	// daşımaması lazımdır (thehackernews-in "-d.png"/"sponsor" filtri).
	ExcludeContains []string `yaml:"exclude_contains,omitempty"`
}

// VideoConfig — embed olunmuş video (YouTube iframe və ya link) axtarışı.
type VideoConfig struct {
	// Mode — "iframe_youtube" (iframe[src*='youtube.com/embed'] axtarır)
	// və ya "anchor_href" (Selector-un özünün href atributunu götürür).
	Mode string `yaml:"mode"`
	// Scope — "iframe_youtube" üçün, iframe-in axtarılacağı konteyner
	// (boşdursa ContentSelector istifadə olunur).
	Scope string `yaml:"scope,omitempty"`
	// Selector — "anchor_href" üçün, href götürüləcək elementin selector-u.
	Selector string `yaml:"selector,omitempty"`
}
