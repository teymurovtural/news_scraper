package middleware

import (
	"crypto/subtle"
	"net/http"
)

// APIKeyAuth — hər sorğuda X-API-Key header-ini yoxlayır.
// API_KEY boşdursa middleware deaktivdir (local dev üçün).
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("X-API-Key")

			// BUG FIX (timing attack): adi `key != apiKey` müqayisəsi
			// Go-da soldan sağa gedir və ilk fərqli hərfdə DAYANIR — yəni
			// düzgün key-ə nə qədər çox oxşayırsa, müqayisə bir o qədər
			// (mikrosaniyələrlə) uzun çəkir. Bu vaxt fərqini statistik
			// ölçərək API key-i hərf-hərf "sındırmaq" nəzəri cəhətdən
			// mümkündür ("timing attack"). subtle.ConstantTimeCompare hər
			// zaman BÜTÜN baytları yoxlayır, cavab vaxtı key-in düzgün/səhv
			// olmasından asılı olmayaraq sabitdir.
			if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
