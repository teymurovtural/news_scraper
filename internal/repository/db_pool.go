package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// dbPool — FeedItemRepository/SourceRepository-nin ehtiyac duyduğu minimal
// DB interfeysi. *pgxpool.Pool bu metodların hamısını artıq təmin edir, ona
// görə mövcud çağıran kodun (cmd/server/main.go və s.) HEÇ BİR YERİ dəyişmir
// — Go interfeysləri strukturaldır, *pgxpool.Pool ötürüləndə avtomatik uyğun
// gəlir.
//
// Bu interfeysin əsas məqsədi TESTLƏRDİR: real Postgres-ə ehtiyac olmadan,
// `pgxmock` kitabxanası ilə DB davranışını simulyasiya edib repository-lərin
// SQL sorğularını (WHERE şərtləri, parametrlərin sırası və s.) yoxlaya bilirik.
type dbPool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}
