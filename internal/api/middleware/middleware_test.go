package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

func TestAPIKeyAuth_EmptyKeyDisablesAuth(t *testing.T) {
	handler := APIKeyAuth("")(okHandler())

	req := httptest.NewRequest("GET", "/anything", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("API_KEY boş olanda auth deaktiv olmalıdır, alındı: %d", rec.Code)
	}
}

func TestAPIKeyAuth_RejectsMissingHeader(t *testing.T) {
	handler := APIKeyAuth("secret123")(okHandler())

	req := httptest.NewRequest("GET", "/anything", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("header-siz sorğu rədd edilməlidir, alındı: %d", rec.Code)
	}
}

func TestAPIKeyAuth_RejectsWrongKey(t *testing.T) {
	handler := APIKeyAuth("secret123")(okHandler())

	req := httptest.NewRequest("GET", "/anything", nil)
	req.Header.Set("X-API-Key", "wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("yanlış key rədd edilməlidir, alındı: %d", rec.Code)
	}
}

func TestAPIKeyAuth_AcceptsCorrectKey(t *testing.T) {
	handler := APIKeyAuth("secret123")(okHandler())

	req := httptest.NewRequest("GET", "/anything", nil)
	req.Header.Set("X-API-Key", "secret123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("doğru key qəbul olunmalıdır, alındı: %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("handler-in cavabı gözlənilən deyil: %q", rec.Body.String())
	}
}

func TestLogger_CapturesActualStatusCode(t *testing.T) {
	// Logger özü stdout-a yazır (log.Printf), bunu birbaşa test etmirik —
	// yoxlanılan əsas şey budur: wrapped responseWriter DƏYİŞDİRİLMİŞ
	// status kodunu düzgün ötürür (məs. handler 404 qaytarsa, client-ə
	// həqiqətən 404 çatmalıdır, Logger-in "default 200" ilə əvəz etməməlidir).
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	handler := Logger(notFoundHandler)

	req := httptest.NewRequest("GET", "/yoxdur", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Logger status kodunu dəyişib: gözlənilən 404, alındı %d", rec.Code)
	}
}
