package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/new-scraper/internal/domain"

	"github.com/jackc/pgx/v5"
)

type FeedItemRepository struct {
	db dbPool
}

func NewFeedItemRepository(db dbPool) *FeedItemRepository {
	return &FeedItemRepository{db: db}
}

func scanFeedItem(row interface {
	Scan(dest ...any) error
}) (domain.FeedItem, error) {
	var item domain.FeedItem
	var author, publishedDate, content, contentHTML, viewURL, videoURL *string
	err := row.Scan(
		&item.ID, &item.SourceID, &item.Title, &item.Link,
		&author, &publishedDate, &content, &contentHTML, &viewURL,
		&item.Images, &videoURL, &item.IsScraped, &item.PublishedAt, &item.FetchedAt, &item.ScrapedAt,
		&item.CVEIDs,
	)
	if err != nil {
		return domain.FeedItem{}, err
	}
	if author != nil {
		item.Author = *author
	}
	if publishedDate != nil {
		item.PublishedDate = *publishedDate
	}
	if content != nil {
		item.Content = *content
	}
	if contentHTML != nil {
		item.ContentHTML = *contentHTML
	}
	if viewURL != nil {
		item.ViewURL = *viewURL
	}
	if videoURL != nil {
		item.VideoURL = *videoURL
	}
	return item, nil
}

const selectFields = `
	SELECT id, source_id, title, link, author, published_date,
	       content, content_html, view_url, images, video_url, is_scraped, published_at, fetched_at, scraped_at,
	       cve_ids
	FROM feed_items`

func (r *FeedItemRepository) Create(ctx context.Context, item *domain.FeedItem) error {
	query := `
		INSERT INTO feed_items (source_id, title, link, published_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (link) DO NOTHING
		RETURNING id, fetched_at
	`

	err := r.db.QueryRow(ctx, query,
		item.SourceID, item.Title, item.Link, item.PublishedAt,
	).Scan(&item.ID, &item.FetchedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrDuplicateItem
		}
		return fmt.Errorf("feed_item_repository: Create: %w", err)
	}

	return nil
}

func (r *FeedItemRepository) UpdateScrapedData(ctx context.Context, id int64, title, author, publishedDate, content, contentHTML, viewURL string, images []domain.ImageItem, videoURL string, cveIDs []string) error {
	query := `
       UPDATE feed_items
       SET title = $1, author = $2, published_date = $3, content = $4, content_html = $5, view_url = $6, images = $7, video_url = $8, cve_ids = $9, is_scraped = true, scraped_at = NOW()
       WHERE id = $10
    `

	_, err := r.db.Exec(ctx, query, title, author, publishedDate, content, contentHTML, viewURL, images, videoURL, cveIDs, id)
	if err != nil {
		return fmt.Errorf("feed_item_repository: UpdateScrapedData: %w", err)
	}

	return nil
}

func (r *FeedItemRepository) GetAll(ctx context.Context, limit, offset int) ([]domain.FeedItem, error) {
	query := selectFields + `
		ORDER BY fetched_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("feed_item_repository: GetAll: %w", err)
	}
	defer rows.Close()

	var items []domain.FeedItem
	for rows.Next() {
		item, err := scanFeedItem(rows)
		if err != nil {
			return nil, fmt.Errorf("feed_item_repository: GetAll scan: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}

func (r *FeedItemRepository) GetUnscraped(ctx context.Context, limit int) ([]domain.FeedItem, error) {
	query := selectFields + `
		WHERE is_scraped = false
		ORDER BY fetched_at ASC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("feed_item_repository: GetUnscraped: %w", err)
	}
	defer rows.Close()

	var items []domain.FeedItem
	for rows.Next() {
		item, err := scanFeedItem(rows)
		if err != nil {
			return nil, fmt.Errorf("feed_item_repository: GetUnscraped scan: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}

func (r *FeedItemRepository) GetEmptyContent(ctx context.Context, limit int) ([]domain.FeedItem, error) {
	// DIZAYN DƏYİŞİKLİYİ: title artıq RSS-dən deyil, scrape mərhələsindən gəlir
	// (fetcher.go artıq title yazmır). Ona görə title boş qalarsa (scrape
	// zamanı title-selector uğursuz olsa), bu, artıq "RSS-dən gələn ehtiyat
	// dəyər" ilə örtülmür — item əbədi başlıqsız qala bilər. title boşluğunu
	// da bura əlavə etdik ki, digər sahələr kimi avtomatik retry olunsun.
	query := selectFields + `
		WHERE is_scraped = true
		  AND (
		    content IS NULL OR content = ''
		    OR author IS NULL OR author = ''
		    OR title IS NULL OR title = ''
		  )
		ORDER BY fetched_at DESC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("feed_item_repository: GetEmptyContent: %w", err)
	}
	defer rows.Close()

	var items []domain.FeedItem
	for rows.Next() {
		item, err := scanFeedItem(rows)
		if err != nil {
			return nil, fmt.Errorf("feed_item_repository: GetEmptyContent scan: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}

func (r *FeedItemRepository) GetByID(ctx context.Context, id int64) (*domain.FeedItem, error) {
	query := selectFields + `
		WHERE id = $1
	`

	item, err := scanFeedItem(r.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrItemNotFound
		}
		return nil, fmt.Errorf("feed_item_repository: GetByID: %w", err)
	}

	return &item, nil
}

func (r *FeedItemRepository) GetBySource(ctx context.Context, sourceID int64, limit, offset int) ([]domain.FeedItem, error) {
	query := selectFields + `
		WHERE source_id = $1
		ORDER BY fetched_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, query, sourceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("feed_item_repository: GetBySource: %w", err)
	}
	defer rows.Close()

	var items []domain.FeedItem
	for rows.Next() {
		item, err := scanFeedItem(rows)
		if err != nil {
			return nil, fmt.Errorf("feed_item_repository: GetBySource scan: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}

func (r *FeedItemRepository) GetBySourceAfterScrapedAt(ctx context.Context, sourceID int64, after time.Time) ([]domain.FeedItem, error) {
	query := selectFields + `
		WHERE source_id = $1
		  AND is_scraped = true
		  AND scraped_at > $2
		ORDER BY scraped_at ASC`

	rows, err := r.db.Query(ctx, query, sourceID, after)
	if err != nil {
		return nil, fmt.Errorf("feed_item_repository: GetBySourceAfterScrapedAt: %w", err)
	}
	defer rows.Close()

	var items []domain.FeedItem
	for rows.Next() {
		item, err := scanFeedItem(rows)
		if err != nil {
			return nil, fmt.Errorf("feed_item_repository: GetBySourceAfterScrapedAt scan: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *FeedItemRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM feed_items`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("feed_item_repository: Count: %w", err)
	}
	return count, nil
}

// GetRelatedByCVE — cveIDs-dən HƏR HANSI BİRİNİ paylaşan (Postgres-in "&&"
// array overlap operatoru — "bu iki massivdə ortaq elementmi var?") məqalələri
// tapır, excludeID-ni (adətən sorğunun özünün item ID-si) nəticədən çıxarır.
//
// Bilərəkdən sources ilə JOIN olunub (source_name üçün) — cve_ids GIN
// indeksi (bax 011_cve_ids.sql) bu sorğunu sürətli edir, çünki && operatoru
// GIN indeksdən istifadə edə bilir (sıralı skan yox).
func (r *FeedItemRepository) GetRelatedByCVE(ctx context.Context, cveIDs []string, excludeID int64, limit int) ([]domain.RelatedFeedItem, error) {
	if len(cveIDs) == 0 {
		return nil, nil
	}

	query := `
		SELECT fi.id, fi.title, s.name, fi.link
		FROM feed_items fi
		JOIN sources s ON s.id = fi.source_id
		WHERE fi.cve_ids && $1 AND fi.id != $2
		ORDER BY fi.published_at DESC NULLS LAST
		LIMIT $3
	`

	rows, err := r.db.Query(ctx, query, cveIDs, excludeID, limit)
	if err != nil {
		return nil, fmt.Errorf("feed_item_repository: GetRelatedByCVE: %w", err)
	}
	defer rows.Close()

	items := make([]domain.RelatedFeedItem, 0)
	for rows.Next() {
		var item domain.RelatedFeedItem
		if err := rows.Scan(&item.ID, &item.Title, &item.SourceName, &item.Link); err != nil {
			return nil, fmt.Errorf("feed_item_repository: GetRelatedByCVE scan: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}
