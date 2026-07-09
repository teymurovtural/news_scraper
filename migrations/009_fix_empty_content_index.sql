-- BUG FIX: 004_indexes.sql-də yaradılan idx_feed_items_empty_content indeksinin
-- şərti (yalnız content boşdursa) GetEmptyContent sorğusu ilə tam üst-üstə düşmür.
-- Sorğu belədir (feed_item_repository.go):
--   WHERE is_scraped = true
--     AND (content IS NULL OR content = '' OR author IS NULL OR author = '')
-- Amma indeks yalnız bunu əhatə edir:
--   WHERE is_scraped = true AND (content IS NULL OR content = '')
--
-- Postgres partial index yalnız öz şərtinin ƏHATƏ etdiyi sorğularda istifadə
-- oluna bilər. Sorğunun şərti indeksinkindən GENİŞ olduğu üçün (author boş,
-- content dolu olan sətirlər), planner bu halda indeksi keçib tam skan edə bilər.
-- İndeksi sorğu ilə tam eyni şərtlə yenidən yaradırıq ki, hər iki halda da işləsin.

DROP INDEX IF EXISTS idx_feed_items_empty_content;

CREATE INDEX IF NOT EXISTS idx_feed_items_empty_content
    ON feed_items(fetched_at DESC)
    WHERE is_scraped = true
        AND (content IS NULL OR content = '' OR author IS NULL OR author = '');