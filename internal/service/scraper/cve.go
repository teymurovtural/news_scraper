package scraper

import (
	"regexp"
	"strings"
)

// cveRegex — "CVE-2026-58644" formatını tapır. Rəsmi format: CVE-<il>-<4-7
// rəqəm>. Case-insensitive edirik ki, "cve-2026-58644" kimi kiçik hərflə
// yazılmış hallar da (nadir, amma mümkündür) tutulsun.
var cveRegex = regexp.MustCompile(`(?i)CVE-\d{4}-\d{4,7}`)

// ExtractCVEIDs — verilmiş mətndən (adətən title+content birləşməsi) bütün
// unikal CVE ID-lərini çıxarır, hər zaman BÖYÜK HƏRFLƏ (kanonik format,
// "CVE-2026-58644") normalize edilmiş şəkildə qaytarır — DB-də eyni CVE-nin
// fərqli case-lərlə iki "fərqli" qiymət kimi saxlanmasının qarşısını alır.
//
// Nəticə sırası mətndə rast gəlmə sırasına uyğundur (deterministik), dublikat
// yoxdur. Tapılmasa boş (amma nil OLMAYAN) slice qaytarır — cve_ids sütunu
// DB-də NOT NULL olduğu üçün nil slice göndərmək UPDATE zamanı SQL NULL-a
// çevrilir və "not-null constraint" xətasına səbəb olur.
func ExtractCVEIDs(text string) []string {
	matches := cveRegex.FindAllString(text, -1)
	ids := make([]string, 0, len(matches))

	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		id := strings.ToUpper(m)
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}
