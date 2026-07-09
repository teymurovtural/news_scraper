package config

import (
	"net/url"
	"strings"
	"testing"
)

func TestConnectionString_EscapesSpecialCharacters(t *testing.T) {
	// BUG FIX regressiya testi: DSN əvvəllər raw fmt.Sprintf ilə qurulurdu —
	// parolda "@" və ya boşluq kimi xüsusi simvol olsa, DSN səhv parse
	// oluna bilərdi. İndi net/url düzgün escape edir.
	c := &DBConfig{
		Host:     "localhost",
		Port:     "5434",
		Name:     "news_scraper",
		User:     "postgres",
		Password: "p@ss word=123",
		SSLMode:  "disable",
	}

	dsn := c.ConnectionString()

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("qurulan DSN özü valid URL deyil: %v (%s)", err, dsn)
	}

	if parsed.Scheme != "postgres" {
		t.Errorf("gözlənilən scheme 'postgres', alındı: %s", parsed.Scheme)
	}
	gotPassword, _ := parsed.User.Password()
	if gotPassword != c.Password {
		t.Errorf("parol decode ediləndə gözlənilən %q, alındı %q", c.Password, gotPassword)
	}
	if parsed.Query().Get("sslmode") != "disable" {
		t.Errorf("sslmode parametri gözlənildiyi kimi deyil: %s", dsn)
	}
}

func TestConnectionString_SimpleValues(t *testing.T) {
	c := &DBConfig{
		Host: "localhost", Port: "5434", Name: "news_scraper",
		User: "postgres", Password: "simple123", SSLMode: "disable",
	}
	dsn := c.ConnectionString()

	if !strings.HasPrefix(dsn, "postgres://") {
		t.Errorf("DSN 'postgres://' ilə başlamalıdır: %s", dsn)
	}
	if !strings.Contains(dsn, "localhost:5434") {
		t.Errorf("host:port DSN-də olmalıdır: %s", dsn)
	}
	if !strings.Contains(dsn, "/news_scraper") {
		t.Errorf("dbname DSN-də olmalıdır: %s", dsn)
	}
}

func TestLoad_MissingRequiredEnvVar_ReturnsError(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DB_HOST", "") // vacib dəyəri boşaldırıq

	_, err := Load()
	if err == nil {
		t.Fatal("DB_HOST boş olanda Load() xəta qaytarmalıdır, qaytarmadı")
	}
	if !strings.Contains(err.Error(), "DB_HOST") {
		t.Errorf("xəta mesajı DB_HOST-a işarə etməlidir, alındı: %v", err)
	}
}

func TestLoad_InvalidPollInterval_ReturnsError(t *testing.T) {
	setValidEnv(t)
	t.Setenv("POLL_INTERVAL_SECONDS", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("POLL_INTERVAL_SECONDS rəqəm olmayanda xəta qaytarmalıdır")
	}
}

func TestLoad_Success(t *testing.T) {
	setValidEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("bütün env var-lar düzgün olanda Load() xəta verməməlidir: %v", err)
	}

	if cfg.DB.Host != "localhost" {
		t.Errorf("DB.Host gözlənildiyi kimi deyil: %s", cfg.DB.Host)
	}
	if cfg.Poller.WorkerCount != 5 {
		t.Errorf("Poller.WorkerCount gözlənildiyi kimi deyil: %d", cfg.Poller.WorkerCount)
	}
	if !cfg.Playwright.Headless {
		t.Errorf("PLAYWRIGHT_HEADLESS=true olanda Headless true olmalıdır")
	}
}

// setValidEnv — hər test üçün bütün lazımi env var-ları düzgün dəyərlərlə
// təyin edir. t.Setenv testdən sonra avtomatik təmizləyir (paralel testlərə
// təsir etmir).
func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5434")
	t.Setenv("DB_NAME", "news_scraper")
	t.Setenv("DB_USER", "postgres")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("SERVER_PORT", "8082")
	t.Setenv("API_KEY", "")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("POLL_INTERVAL_SECONDS", "900")
	t.Setenv("WORKER_COUNT", "5")
	t.Setenv("PLAYWRIGHT_HEADLESS", "true")
}
