package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/new-scraper/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

var feedItemColumns = []string{
	"id", "source_id", "title", "link", "author", "published_date",
	"content", "content_html", "view_url", "images", "video_url",
	"is_scraped", "published_at", "fetched_at", "scraped_at", "cve_ids",
}

func TestGetByID_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("mock pool yaradılmadı: %v", err)
	}
	defer mock.Close()

	author := "Test Author"
	fetchedAt := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	rows := pgxmock.NewRows(feedItemColumns).AddRow(
		int64(77), int64(1), "Test Title", "https://example.com/a",
		&author, (*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil),
		[]domain.ImageItem{}, (*string)(nil), true,
		(*time.Time)(nil), fetchedAt, (*time.Time)(nil), []string{},
	)

	mock.ExpectQuery("SELECT").WithArgs(int64(77)).WillReturnRows(rows)

	repo := NewFeedItemRepository(mock)
	item, err := repo.GetByID(context.Background(), 77)
	if err != nil {
		t.Fatalf("gözlənilməz xəta: %v", err)
	}

	if item.ID != 77 {
		t.Errorf("ID: gözlənilən 77, alındı %d", item.ID)
	}
	if item.Title != "Test Title" {
		t.Errorf("Title: gözlənilən 'Test Title', alındı %q", item.Title)
	}
	if item.Author != "Test Author" {
		t.Errorf("Author: gözlənilən 'Test Author', alındı %q", item.Author)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("gözlənilən sorğular icra olunmayıb: %v", err)
	}
}

// TestGetByID_NotFound — DB-də sətir tapılmayanda domain.ErrItemNotFound
// qaytarılmalıdır (pgx.ErrNoRows-un düzgün map olunması).
func TestGetByID_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("mock pool yaradılmadı: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery("SELECT").WithArgs(int64(999)).WillReturnError(pgx.ErrNoRows)

	repo := NewFeedItemRepository(mock)
	_, err = repo.GetByID(context.Background(), 999)

	if !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("gözlənilən domain.ErrItemNotFound, alındı: %v", err)
	}
}

// TestUpdateScrapedData_PassesTitleParameter — bu, söhbətdə etdiyimiz dizayn
// dəyişikliyinin (title artıq RSS-dən yox, scrape mərhələsindən yazılır)
// regressiya testidir: UpdateScrapedData çağırılanda title DƏQİQ ötürülən
// dəyər kimi SQL-ə getməlidir.
func TestUpdateScrapedData_PassesTitleParameter(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("mock pool yaradılmadı: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec("UPDATE feed_items").
		WithArgs("Yeni Başlıq", "Müəllif", "2026-07-08", "content", "<p>html</p>", "http://view", []domain.ImageItem(nil), "video-url", []string{"CVE-2026-58644"}, int64(42)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	repo := NewFeedItemRepository(mock)
	err = repo.UpdateScrapedData(context.Background(), 42,
		"Yeni Başlıq", "Müəllif", "2026-07-08", "content", "<p>html</p>",
		"http://view", nil, "video-url", []string{"CVE-2026-58644"},
	)
	if err != nil {
		t.Fatalf("gözlənilməz xəta: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("gözlənilən sorğu icra olunmayıb (title parametri düzgün ötürülməyib ola bilər): %v", err)
	}
}

// TestGetEmptyContent_QueryChecksTitleToo — dizayn dəyişikliyinin digər
// tərəfi: title RSS-dən gəlmədiyi üçün, scrape uğursuz olub title boş
// qalsa, retry sorğusu bunu da tutmalıdır (yalnız content/author yox).
func TestGetEmptyContent_QueryChecksTitleToo(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("mock pool yaradılmadı: %v", err)
	}
	defer mock.Close()

	// Sorğu mətnində "title" sözünün olduğunu tələb edirik — QueryMatcher
	// regex-i "title" alt-sətrini axtarır.
	mock.ExpectQuery(`(?s)SELECT.*WHERE.*title`).WithArgs(50).
		WillReturnRows(pgxmock.NewRows(feedItemColumns))

	repo := NewFeedItemRepository(mock)
	_, err = repo.GetEmptyContent(context.Background(), 50)
	if err != nil {
		t.Fatalf("gözlənilməz xəta: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("GetEmptyContent sorğusu title-ı yoxlamır (dizayn dəyişikliyi geri qalıb?): %v", err)
	}
}
