-- is_scraped = false olan sətirləri tez tapmaq üçün
-- Partial index: yalnız scrape gözləyən sətirləri indeksləyir
-- Cədvəl böyüdükcə GetUnscraped və GetEmptyContent sorğuları sürətlənir
CREATE INDEX IF NOT EXISTS idx_feed_items_unscraped
    ON feed_items(fetched_at ASC)
    WHERE is_scraped = false;

-- is_scraped = true amma content boş olanlar üçün (retryEmptyContent)
CREATE INDEX IF NOT EXISTS idx_feed_items_empty_content
    ON feed_items(fetched_at DESC)
    WHERE is_scraped = true AND (content IS NULL OR content = '');