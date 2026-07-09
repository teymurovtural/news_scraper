-- DIZAYN DƏYİŞİKLİYİ: title artıq RSS-dən deyil, scrape mərhələsindən gəlir.
-- GetEmptyContent sorğusu indi "title IS NULL OR title = ''" şərtini də yoxlayır
-- (bax feed_item_repository.go). 009-da yaradılan indeks bunu əhatə etmir,
-- ona görə eyni partial-index-uyğunsuzluğu (bax 009-un öz şərhi) yenidən yaranır.
-- İndeksi sorğu ilə tam eyni şərtlə yenidən qururuq.

DROP INDEX IF EXISTS idx_feed_items_empty_content;

CREATE INDEX IF NOT EXISTS idx_feed_items_empty_content
    ON feed_items(fetched_at DESC)
    WHERE is_scraped = true
        AND (
              content IS NULL OR content = ''
                  OR author IS NULL OR author = ''
                  OR title IS NULL OR title = ''
              );