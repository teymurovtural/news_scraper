CREATE TABLE IF NOT EXISTS sources (
                                       id             BIGSERIAL PRIMARY KEY,
                                       name           VARCHAR(255) NOT NULL,
                                       site_url       TEXT NOT NULL,
                                       feed_url       TEXT NOT NULL UNIQUE,
                                       category       VARCHAR(100),
                                       is_active      BOOLEAN NOT NULL DEFAULT true,
                                       last_polled_at TIMESTAMPTZ,
                                       poll_interval  INTEGER NOT NULL DEFAULT 900,
                                       fail_count     INTEGER NOT NULL DEFAULT 0,
                                       created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS feed_items (
                                          id             BIGSERIAL PRIMARY KEY,
                                          source_id      BIGINT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
                                          title          TEXT NOT NULL,
                                          link           TEXT NOT NULL UNIQUE,
                                          author         TEXT,
                                          published_date TEXT,
                                          content        TEXT,
                                          images         JSONB NOT NULL DEFAULT '[]',
                                          is_scraped     BOOLEAN NOT NULL DEFAULT false,
                                          published_at   TIMESTAMPTZ,
                                          fetched_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_feed_items_source_id    ON feed_items(source_id);
CREATE INDEX IF NOT EXISTS idx_feed_items_published_at ON feed_items(published_at DESC);