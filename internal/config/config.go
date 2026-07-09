package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DB         DBConfig
	Server     ServerConfig
	Log        LogConfig
	Poller     PollerConfig
	Playwright PlaywrightConfig
}

type PlaywrightConfig struct {
	Headless bool
}

type DBConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

type ServerConfig struct {
	Port   string
	APIKey string
}

type LogConfig struct {
	Level  string
	Format string // "text" (default, terminalda oxunaqlı) və ya "json" (log toplayıcı alətlər üçün)
}

type PollerConfig struct {
	IntervalSeconds int
	WorkerCount     int
}

func Load() (*Config, error) {
	_ = godotenv.Load() // .env yoxdursa system env var-lar istifadə edilir (Docker)

	pollInterval, err := strconv.Atoi(os.Getenv("POLL_INTERVAL_SECONDS"))
	if err != nil {
		return nil, fmt.Errorf("config: POLL_INTERVAL_SECONDS səhvdir: %w", err)
	}

	workerCount, err := strconv.Atoi(os.Getenv("WORKER_COUNT"))
	if err != nil {
		return nil, fmt.Errorf("config: WORKER_COUNT səhvdir: %w", err)
	}

	cfg := &Config{
		Playwright: PlaywrightConfig{
			Headless: os.Getenv("PLAYWRIGHT_HEADLESS") == "true",
		},
		DB: DBConfig{
			Host:     os.Getenv("DB_HOST"),
			Port:     os.Getenv("DB_PORT"),
			Name:     os.Getenv("DB_NAME"),
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			SSLMode:  os.Getenv("DB_SSLMODE"),
		},
		Server: ServerConfig{
			Port:   os.Getenv("SERVER_PORT"),
			APIKey: os.Getenv("API_KEY"),
		},
		Log: LogConfig{
			Level:  os.Getenv("LOG_LEVEL"),
			Format: logFormatOrDefault(os.Getenv("LOG_FORMAT")),
		},
		Poller: PollerConfig{
			IntervalSeconds: pollInterval,
			WorkerCount:     workerCount,
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// logFormatOrDefault — LOG_FORMAT env dəyişəni boşdursa və ya tanınmayan
// dəyərdirsə, təhlükəsiz default olaraq "text" qaytarır (dev mühitində
// oxunaqlıdır). Production-da explisit "json" təyin edilməlidir.
// "pretty" — inkişaf zamanı emoji + rəngli çıxış üçün (bax platform/logger).
func logFormatOrDefault(v string) string {
	switch v {
	case "json":
		return "json"
	case "pretty":
		return "pretty"
	default:
		return "text"
	}
}

func (c *Config) validate() error {
	required := map[string]string{
		"DB_HOST":     c.DB.Host,
		"DB_PORT":     c.DB.Port,
		"DB_NAME":     c.DB.Name,
		"DB_USER":     c.DB.User,
		"DB_PASSWORD": c.DB.Password,
		"SERVER_PORT": c.Server.Port,
	}

	for key, val := range required {
		if val == "" {
			return fmt.Errorf("config: %s mütləq doldurulmalıdır", key)
		}
	}

	return nil
}

func (c *DBConfig) ConnectionString() string {
	// BUG FIX: əvvəlki versiya "host=%s ... password=%s ..." formatını raw
	// fmt.Sprintf ilə qururdu — əgər DB_PASSWORD (və ya digər sahələr) boşluq,
	// "=", ya da tək dırnaq kimi xüsusi simvol daşısaydı, DSN sətri səhv
	// parse oluna bilərdi. İndi net/url ilə URL-formatlı DSN qururuq (pgx bunu
	// da dəstəkləyir) — url.UserPassword avtomatik olaraq lazımi escape-i edir.
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   fmt.Sprintf("%s:%s", c.Host, c.Port),
		Path:   "/" + c.Name,
	}

	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	u.RawQuery = q.Encode()

	return u.String()
}
