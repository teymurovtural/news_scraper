ALTER TABLE feed_items ADD COLUMN IF NOT EXISTS scraped_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_feed_items_scraped_at ON feed_items(scraped_at);