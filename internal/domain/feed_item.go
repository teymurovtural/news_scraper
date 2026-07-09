package domain

import "time"

type ImageItem struct {
	URL string `json:"url"`
	Alt string `json:"alt,omitempty"`
}

type FeedItem struct {
	ID            int64       `json:"id"`
	SourceID      int64       `json:"source_id"`
	Title         string      `json:"title"`
	Link          string      `json:"link"`
	Author        string      `json:"author"`
	PublishedDate string      `json:"published_date"`
	Content       string      `json:"content"`
	ContentHTML   string      `json:"content_html,omitempty"`
	ViewURL       string      `json:"view_url,omitempty"`
	Images        []ImageItem `json:"images"`
	VideoURL      string      `json:"video_url,omitempty"`
	IsScraped     bool        `json:"is_scraped"`
	PublishedAt   *time.Time  `json:"published_at"`
	FetchedAt     time.Time   `json:"fetched_at"`
	ScrapedAt     *time.Time  `json:"scraped_at"`
}
