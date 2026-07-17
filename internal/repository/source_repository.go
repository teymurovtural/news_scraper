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
	// DİZAYN DƏYİŞİKLİYİ: əvvəllər bu sorğu "fail_count = 0" da edirdi —
	// yəni RSS fetch-i uğurlu olan kimi sayğac sıfırlanırdı, HANSI KI scrape
	// mərhələsinin (Playwright) nəticəsindən tamamilə xəbərsiz idi. Nəticədə:
	// RSS işləyir, amma sayt HTML strukturunu dəyişdiyi üçün HƏR məqalənin
	// scrape-i uğursuz olsa belə, fail_count hər 15 dəqiqədə bir yenidən 0-a
	// düşürdü — 20-lik həddə heç vaxt çatmırdı, mənbə "sağlam" görünməyə
	// davam edirdi, halbuki heç bir yeni content faktiki gəlmirdi.
	//
	// İndi fail_count YALNIZ real content axını təsdiqlənəndə (bir item
	// uğurla scrape olunanda, bax scraper_service.go-dakı ResetFailCount
	// çağırışı) sıfırlanır. RSS-in sadəcə cavab verməsi artıq "sağlamlıq"
	// siqnalı sayılmır.
	query := `
		UPDATE sources
		SET last_polled_at = NOW()
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

// IncrementFailCount — fail_count-u 1 artırır, 20-yə çatsa mənbəni
// avtomatik deaktiv edir (bax domain/repositories.go-dakı şərh).
//
// BUG FIX / YENİ QAYIDIŞ DƏYƏRİ: əvvəllər bu funksiya yalnız error
// qaytarırdı — çağıran tərəf mənbənin məhz BU çağırışla deaktiv olduğunu
// heç vaxt bilmirdi (sükutla baş verirdi, heç bir log/xəbərdarlıq yox idi).
// İndi UPDATE-in RETURNING bəndi ilə, DƏYİŞİKLİKDƏN SONRAKI is_active
// dəyərini birbaşa geri oxuyuruq — çağıran (fetcher.go, scraper_service.go)
// bunu "əvvəl aktiv idi, indi deaktiv oldu" anını bir dəfəlik XƏBƏRDARLIQ
// loglamaq üçün istifadə edir.
func (r *SourceRepository) IncrementFailCount(ctx context.Context, id int64) (bool, error) {
	query := `
		UPDATE sources
		SET fail_count = fail_count + 1,
		    is_active = CASE WHEN fail_count + 1 >= 20 THEN false ELSE is_active END
		WHERE id = $1
		RETURNING is_active
	`

	var isActive bool
	err := r.db.QueryRow(ctx, query, id).Scan(&isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.ErrSourceNotFound
		}
		return false, fmt.Errorf("source_repository: IncrementFailCount: %w", err)
	}

	// isActive=false bu andan sonrakı vəziyyətdir — deməli məhz bu çağırış
	// mənbəni deaktiv etdi (əgər artıq deaktiv idisə, CASE şərti heç işə
	// düşməzdi, çünki fail_count onsuz da artıq görünməzdi — GetActive
	// belə mənbələri ümumiyyətlə qaytarmır, ona görə IncrementFailCount
	// praktikada yalnız hələ aktiv olan mənbələr üçün çağırılır).
	return !isActive, nil
}

// ResetFailCount — fail_count-u sıfırlayır. Bax domain/repositories.go-dakı
// şərh: bu, RSS-fetch uğurundan DEYİL, yalnız real scrape uğurundan sonra
// çağırılmalıdır.
func (r *SourceRepository) ResetFailCount(ctx context.Context, id int64) error {
	query := `
		UPDATE sources
		SET fail_count = 0
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("source_repository: ResetFailCount: %w", err)
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

// Activate — Deactivate-in əksi (bax domain/repositories.go-dakı şərh).
// fail_count-u da sıfırlayır ki, mənbə "təzə başlanğıcla" geri qayıtsın —
// əks halda, əl ilə aktivləşdirilən kimi, hələ 20-ə yaxın qalmış köhnə
// fail_count DB-də qalardı və bir-iki yeni uğursuzluqla YENİDƏN avtomatik
// deaktiv olardı.
func (r *SourceRepository) Activate(ctx context.Context, id int64) error {
	query := `
		UPDATE sources
		SET is_active = true, fail_count = 0
		WHERE id = $1
	`

	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("source_repository: Activate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSourceNotFound
	}

	return nil
}
