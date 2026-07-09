package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var commonFeedPaths = []string{
	"/feed",
	"/feed/",
	"/rss",
	"/rss/",
	"/rss.xml",
	"/feed.xml",
	"/atom.xml",
	"/index.xml",
	"/blog/feed",
	"/blog/rss",
	"/news/feed",
	"/news/rss",
}

type DiscoveryService struct {
	client *http.Client
}

func NewDiscoveryService() *DiscoveryService {
	return &DiscoveryService{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *DiscoveryService) Discover(ctx context.Context, siteURL string) (string, error) {
	if !strings.HasPrefix(siteURL, "http") {
		siteURL = "https://" + siteURL
	}

	// Addım 1 — HTML-dən tap
	feedURL, err := s.discoverFromHTML(ctx, siteURL)
	if err == nil {
		return feedURL, nil
	}

	// Addım 2 — Standart yolları yoxla
	feedURL, err = s.discoverFromPaths(ctx, siteURL)
	if err == nil {
		return feedURL, nil
	}

	return "", fmt.Errorf("discovery: [%s] üçün RSS tapılmadı", siteURL)
}

func (s *DiscoveryService) discoverFromHTML(ctx context.Context, siteURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, siteURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; NewsScraper/1.0)")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	var feedURL string
	doc.Find(`link[type="application/rss+xml"], link[type="application/atom+xml"]`).Each(func(i int, sel *goquery.Selection) {
		if feedURL != "" {
			return
		}
		href, exists := sel.Attr("href")
		if exists && href != "" {
			feedURL = href
		}
	})

	if feedURL == "" {
		return "", fmt.Errorf("HTML-də RSS tapılmadı")
	}

	// Relative URL-i absolute et
	if !strings.HasPrefix(feedURL, "http") {
		base, err := url.Parse(siteURL)
		if err != nil {
			return "", err
		}
		ref, err := url.Parse(feedURL)
		if err != nil {
			return "", err
		}
		feedURL = base.ResolveReference(ref).String()
	}

	return feedURL, nil
}

func (s *DiscoveryService) discoverFromPaths(ctx context.Context, siteURL string) (string, error) {
	base := strings.TrimRight(siteURL, "/")

	for _, path := range commonFeedPaths {
		candidate := base + path

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
		if err != nil {
			continue
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; NewsScraper/1.0)")

		resp, err := s.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			ct := resp.Header.Get("Content-Type")
			if strings.Contains(ct, "xml") || strings.Contains(ct, "rss") || strings.Contains(ct, "atom") {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("heç bir standart yolda RSS tapılmadı")
}
