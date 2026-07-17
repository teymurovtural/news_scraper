package scraper

import (
	"reflect"
	"testing"
)

func TestExtractCVEIDs(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "tək CVE",
			text: "CISA Adds Exploited SharePoint RCE Zero-Day CVE-2026-58644 to KEV",
			want: []string{"CVE-2026-58644"},
		},
		{
			name: "bir neçə CVE",
			text: "Microsoft Patches Record 622 Flaws, including CVE-2026-58644 and CVE-2026-58645, both critical",
			want: []string{"CVE-2026-58644", "CVE-2026-58645"},
		},
		{
			name: "dublikat CVE bir dəfə qaytarılır",
			text: "CVE-2026-58644 was disclosed. Later, CVE-2026-58644 was added to KEV.",
			want: []string{"CVE-2026-58644"},
		},
		{
			name: "kiçik hərflə yazılmış CVE böyük hərfə normalize olunur",
			text: "researchers found cve-2026-12345 in the wild",
			want: []string{"CVE-2026-12345"},
		},
		{
			name: "CVE yoxdursa boş slice qaytarır (nil yox)",
			text: "Risk Ledger Raises $32 Million in Series B Funding",
			want: []string{},
		},
		{
			name: "boş mətn",
			text: "",
			want: []string{},
		},
		{
			name: "qısa rəqəm hissəsi (4 rəqəm)",
			text: "Tracked as CVE-2026-1234, the flaw allows RCE",
			want: []string{"CVE-2026-1234"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCVEIDs(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractCVEIDs(%q) = %v, gözlənilən %v", tt.text, got, tt.want)
			}
		})
	}
}
