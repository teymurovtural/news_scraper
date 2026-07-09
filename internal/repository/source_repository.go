package repository

import (
	"context"
	"errors"
	"fmt"

	"example.com/new-scraper/internal/domain"

	"github.com/jackc/pgx/v5"
)

type SourceRepository struct {
	db dbPool
}

func NewSourceRepository(db dbPool) *SourceRepository {
	return &SourceRepository{db: db}
}

// REFACTOR: GetAll/GetActive/GetByID əvvəllər eyni SELECT sütun siyahısını
// və eyni Scan(...) bloğunu 3 dəfə hərfi-hərfinə təkrarlayırdı (kod təkrarı,
// feed_item_repository.go-dakı selectFields/scanFeedItem pattern-inin
// bənzəri buraya tətbiq olunmamışdı). İndi ortaq sütun siyahısı və scan
// funksiyası bir yerdə saxlanılır — sabah yeni sütun əlavə etsən, yalnız
// BURADA dəyişmək kifayət edər, 3 yerdə yox.
const selectSourceFields = `
	SELECT id, name, site_url, feed_url, category, is_active,
	       last_polled_at, poll_interval, fail_count, created_at, last_exported_at
	FROM sources
`

// scanSource — həm rows.Next() daxilində, həm də QueryRow üçün ortaq scan
// funksiyası. `row` parametri həm pgx.Rows (Query nəticəsi), həm də
// pgx.Row (QueryRow nəticəsi) interfeysini qəbul edir — hər ikisində Scan
// metodu eyni imzaya malikdir.
func scanSource(row interface{ Scan(dest ...any) error }) (domain.Source, error) {
	var s domain.Source
	err := row.Scan(
		&s.ID, &s.Name, &s.SiteURL, &s.FeedURL, &s.Category,
		&s.IsActive, &s.LastPolledAt, &s.PollInterval,
		&s.FailCount, &s.CreatedAt, &s.LastExportedAt,
	)
	return s, err
}

func (r *SourceRepository) GetAll(ctx context.Context) ([]domain.Source, error) {
	query := selectSourceFields + `ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("source_repository: GetAll: %w", err)
	}
	defer rows.Close()

	var sources []domain.Source
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("source_repository: GetAll scan: %w", err)
		}
		sources = append(sources, s)
	}

	return sources, nil
}

func (r *SourceRepository) GetActive(ctx context.Context) ([]domain.Source, error) {
	query := selectSourceFields + `
		WHERE is_active = true
		ORDER BY last_polled_at ASC NULLS FIRST
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("source_repository: GetActive: %w", err)
	}
	defer rows.Close()

	var sources []domain.Source
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("source_repository: GetActive scan: %w", err)
		}
		sources = append(sources, s)
	}

	return sources, nil
}

func (r *SourceRepository) GetByID(ctx context.Context, id int64) (*domain.Source, error) {
	query := selectSourceFields + `WHERE id = $1`

	s, err := scanSource(r.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSourceNotFound
		}
		return nil, fmt.Errorf("source_repository: GetByID: %w", err)
	}

	return &s, nil
}

func (r *SourceRepository) Create(ctx context.Context, s *domain.Source) error {
	query := `
		INSERT INTO sources (name, site_url, feed_url, category, poll_interval)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (feed_url) DO NOTHING
		RETURNING id, created_at
	`

	err := r.db.QueryRow(ctx, query,
		s.Name, s.SiteURL, s.FeedURL, s.Category, s.PollInterval,
	).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrDuplicateSource
		}
		return fmt.Errorf("source_repository: Create: %w", err)
	}

	return nil
}

func (r *SourceRepository) UpdateLastPolled(ctx context.Context, id int64) error {
	query := `
		UPDATE sources
		SET last_polled_at = NOW(), fail_count = 0
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("source_repository: UpdateLastPolled: %w", err)
	}

	return nil
}

func (r *SourceRepository) UpdateLastExportedAt(ctx context.Context, id int64) error {
	query := `
		UPDATE sources
		SET last_exported_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("source_repository: UpdateLastExportedAt: %w", err)
	}

	return nil
}

func (r *SourceRepository) IncrementFailCount(ctx context.Context, id int64) error {
	query := `
		UPDATE sources
		SET fail_count = fail_count + 1,
		    is_active = CASE WHEN fail_count + 1 >= 20 THEN false ELSE is_active END
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("source_repository: IncrementFailCount: %w", err)
	}

	return nil
}

// Deactivate — mənbəni "soft delete" edir (bax domain/repositories.go-dakı
// şərh: niyə hard delete yoxdur). Sətir tapılmasa (id yanlışdırsa),
// domain.ErrSourceNotFound qaytarılır ki, handler bunu 404-ə çevirə bilsin.
func (r *SourceRepository) Deactivate(ctx context.Context, id int64) error {
	query := `
		UPDATE sources
		SET is_active = false
		WHERE id = $1
	`

	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("source_repository: Deactivate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSourceNotFound
	}

	return nil
}
