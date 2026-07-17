-- Bir CVE-nin 2+ mənbədə yazılıb-yazılmadığını GÖRÜNMƏ ANINDA (siyahıda,
-- əlavə sorğu etmədən) bilmək üçün. Bu sahə hesablanmış (derived) data-dır —
-- əsl həqiqət mənbəyi hər zaman cve_ids-dir, bu sahə yalnız "sürətli baxış"
-- üçün keşdir. Scrape zamanı (internal/service/scraper/scraper_service.go
-- -dakı UpdateRelatedCVEFlags çağırışı ilə) geriyə-dönük yenilənir: yeni bir
-- CVE tapılanda, onu paylaşan BÜTÜN item-lərin (köhnə + yeni) bayrağı
-- yenidən hesablanır.
ALTER TABLE feed_items ADD COLUMN IF NOT EXISTS has_related_cve BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_feed_items_has_related_cve ON feed_items(has_related_cve) WHERE has_related_cve = true;