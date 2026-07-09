package exporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestAppendToFile_DedupsBylLink — söhbətdə tapılan dublikat bug-ının
// regressiya testidir: eyni Link-ə malik item YENIDƏN göndərilsə (məs. DB
// sıfırlanıb təkrar scrape ediləndə), fayla İKİNCİ dəfə əlavə olunmamalıdır.
func TestAppendToFile_DedupsByLink(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "export_test.json")

	// 1-ci "sessiya"
	added1, err := appendToFile(file, []ExportItem{
		{ID: 1, Link: "https://example.com/a", Title: "A"},
		{ID: 2, Link: "https://example.com/b", Title: "B"},
	})
	if err != nil {
		t.Fatalf("1-ci yazma xətası: %v", err)
	}
	if added1 != 2 {
		t.Errorf("1-ci sessiya: gözlənilən 2 əlavə, alındı %d", added1)
	}

	// 2-ci "sessiya" — CRASH SİMULYASİYASI: DB sıfırlanıb, eyni linklər
	// YENİ ID-lərlə (auto-increment yenidən 1-dən başlayıb) qayıdır +
	// 1 həqiqətən yeni məqalə.
	added2, err := appendToFile(file, []ExportItem{
		{ID: 1, Link: "https://example.com/a", Title: "A-YENIDEN"}, // eyni link, fərqli ID
		{ID: 2, Link: "https://example.com/b", Title: "B-YENIDEN"}, // eyni link, fərqli ID
		{ID: 3, Link: "https://example.com/c", Title: "C"},         // əsl yeni
	})
	if err != nil {
		t.Fatalf("2-ci yazma xətası: %v", err)
	}
	if added2 != 1 {
		t.Errorf("2-ci sessiya: gözlənilən YALNIZ 1 əlavə (C), alındı %d — dedup işləmir", added2)
	}

	// Faylın son vəziyyəti: cəmi 3 unikal item olmalıdır, dublikat yox.
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("fayl oxunmadı: %v", err)
	}
	var final []ExportItem
	if err := json.Unmarshal(data, &final); err != nil {
		t.Fatalf("JSON parse xətası: %v", err)
	}
	if len(final) != 3 {
		t.Fatalf("gözlənilən 3 unikal item, faylda %d var: %+v", len(final), final)
	}

	seen := make(map[string]bool)
	for _, item := range final {
		if seen[item.Link] {
			t.Errorf("DUBLİKAT LİNK FAYLDA: %s", item.Link)
		}
		seen[item.Link] = true
	}
}

// TestAppendToFile_NoNewItems_ReturnsZeroWithoutTouchingFile — heç bir yeni
// item olmadıqda fayl heç toxunulmamalıdır (lazımsız disk yazısı olmasın).
func TestAppendToFile_NoNewItems_ReturnsZeroWithoutTouchingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "export_test.json")

	if _, err := appendToFile(file, []ExportItem{
		{ID: 1, Link: "https://example.com/a", Title: "A"},
	}); err != nil {
		t.Fatalf("ilk yazma xətası: %v", err)
	}

	info1, _ := os.Stat(file)

	added, err := appendToFile(file, []ExportItem{
		{ID: 1, Link: "https://example.com/a", Title: "A"}, // artıq var
	})
	if err != nil {
		t.Fatalf("2-ci çağırış xətası: %v", err)
	}
	if added != 0 {
		t.Errorf("gözlənilən 0 əlavə, alındı %d", added)
	}

	info2, _ := os.Stat(file)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("fayl lazımsız yerə yenidən yazılıb (heç bir yeni item olmasa belə)")
	}
}
