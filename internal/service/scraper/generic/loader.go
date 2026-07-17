package generic

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// configFile — scraper_configs.yaml-ın kök strukturu (sources.yaml-dakı
// SourcesFile pattern-i ilə eynidir, bax cmd/sync_sources/main.go).
type configFile struct {
	Sources []SourceConfig `yaml:"sources"`
}

// LoadConfigs — verilmiş YAML fayldan bütün mənbə scraper konfiqurasiyalarını
// oxuyur və minimal validasiya edir (prefix/content_selector boş olmamalıdır
// — bunlar olmadan scraper heç işə düşə bilməz).
func LoadConfigs(path string) ([]SourceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("generic: scraper konfiqurasiyası oxunmadı (%s): %w", path, err)
	}

	var cf configFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("generic: scraper konfiqurasiyası parse edilmədi (%s): %w", path, err)
	}

	for i, sc := range cf.Sources {
		if sc.Prefix == "" {
			return nil, fmt.Errorf("generic: sources[%d] üçün 'prefix' boşdur", i)
		}
		if sc.Name == "" {
			return nil, fmt.Errorf("generic: sources[%d] (prefix=%s) üçün 'name' boşdur", i, sc.Prefix)
		}
		if sc.ContentSelector == "" {
			return nil, fmt.Errorf("generic: sources[%d] (%s) üçün 'content_selector' boşdur", i, sc.Name)
		}
	}

	return cf.Sources, nil
}
