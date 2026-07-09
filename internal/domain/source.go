package domain

import "time"

type Source struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	SiteURL        string     `json:"site_url"`
	FeedURL        string     `json:"feed_url"`
	Category       string     `json:"category"`
	IsActive       bool       `json:"is_active"`
	LastPolledAt   *time.Time `json:"last_polled_at"`
	PollInterval   int        `json:"poll_interval"`
	FailCount      int        `json:"fail_count"`
	CreatedAt      time.Time  `json:"created_at"`
	LastExportedAt *time.Time `json:"last_exported_at"`
}
