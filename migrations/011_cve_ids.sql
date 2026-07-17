-- CVE ID-lərini (məs. "CVE-2026-58644") saxlayır — bir məqalədə bir neçə
-- CVE ola bilər (məs. "Patch Tuesday: 622 flaws" kimi xülasə məqalələr).
-- Native Postgres text[] istifadə olunur (JSONB yox) — sadə string massivi
-- üçün ən yüngül seçim, GIN indeksi ilə "eyni CVE-ni paylaşan məqalələr"
-- sorğusunu (gələcək əlaqələndirmə addımı) effektiv edir.
ALTER TABLE feed_items ADD COLUMN IF NOT EXISTS cve_ids TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_feed_items_cve_ids ON feed_items USING GIN (cve_ids);