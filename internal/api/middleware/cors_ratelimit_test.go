package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_EmptyOriginsDisablesHeaders(t *testing.T) {
	handler := CORS(nil)(okHandler())

	req := httptest.NewRequest("GET", "/anything", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("boş allowedOrigins ilə heç bir CORS header əlavə olunmamalıdır, alındı: %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("gözlənilən 200, alındı: %d", rec.Code)
	}
}

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	handler := CORS([]string{"https://dashboard.example.com"})(okHandler())

	req := httptest.NewRequest("GET", "/anything", nil)
	req.Header.Set("Origin", "https://dashboard.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://dashboard.example.com" {
		t.Errorf("icazə verilən origin üçün header düzgün qaytarılmalıdır, alındı: %q", got)
	}
}

func TestCORS_RejectsUnlistedOrigin(t *testing.T) {
	handler := CORS([]string{"https://dashboard.example.com"})(okHandler())

	req := httptest.NewRequest("GET", "/anything", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("siyahıda olmayan origin üçün header əlavə olunmamalıdır, alındı: %q", got)
	}
}

func TestCORS_HandlesPreflightWithoutReachingHandler(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := CORS([]string{"*"})(next)

	req := httptest.NewRequest("OPTIONS", "/api/v1/items", nil)
	req.Header.Set("Origin", "https://dashboard.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("OPTIONS preflight əsl handler-ə çatmamalıdır")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight üçün gözlənilən 204, alındı: %d", rec.Code)
	}
}

func TestRateLimit_AllowsWithinBurst(t *testing.T) {
	handler := RateLimit(60, 3)(okHandler())

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/anything", nil)
		req.RemoteAddr = "1.2.3.4:5555"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("burst daxilindəki %d-ci sorğu icazəli olmalıdır, alındı: %d", i+1, rec.Code)
		}
	}
}

func TestRateLimit_RejectsOverBurst(t *testing.T) {
	handler := RateLimit(60, 2)(okHandler())

	req := httptest.NewRequest("GET", "/anything", nil)
	req.RemoteAddr = "9.9.9.9:1111"

	// İlk 2 sorğu (burst=2) keçməlidir
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("burst daxilindəki sorğu keçməlidir, alındı: %d", rec.Code)
		}
	}

	// 3-cü sorğu bucket boş olduğu üçün rədd edilməlidir
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("burst-dən sonrakı sorğu 429 qaytarmalıdır, alındı: %d", rec.Code)
	}
}

func TestRateLimit_DifferentIPsHaveIndependentBuckets(t *testing.T) {
	handler := RateLimit(60, 1)(okHandler())

	req1 := httptest.NewRequest("GET", "/anything", nil)
	req1.RemoteAddr = "1.1.1.1:1111"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("1-ci IP-nin ilk sorğusu keçməlidir, alındı: %d", rec1.Code)
	}

	req2 := httptest.NewRequest("GET", "/anything", nil)
	req2.RemoteAddr = "2.2.2.2:2222"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("fərqli IP-nin öz bucket-i olmalıdır (əvvəlkindən asılı olmamalıdır), alındı: %d", rec2.Code)
	}
}
