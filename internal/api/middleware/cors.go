package middleware

import "net/http"

// CORS — brauzer-based frontend-lərin (gələcək Cyberattacks/Vulnerability
// Dashboard) bu API-yə çarpaz-domen (cross-origin) sorğu göndərə bilməsi
// üçün lazımi Access-Control-* header-lərini əlavə edir.
//
// DEFAULT DAVRANIŞ (allowedOrigins boşdursa): heç bir header əlavə
// olunmur — bu, mövcud davranışla TAM eynidir (backward-compatible).
// CORS yalnız CORS_ALLOWED_ORIGINS env dəyişəni açıq təyin ediləndə aktiv olur.
//
// "*" xüsusi dəyəri bütün origin-lərə icazə verir. Bu, credential-based
// (cookie) autentifikasiya ilə BİRGƏ istifadə olunanda təhlükəli ola bilər,
// amma bu API cookie yox, custom "X-API-Key" header-i ilə autentifikasiya
// olunur — ona görə "*" bu kontekstdə təhlükəsizdir (brauzer credential-lı
// sorğularda "*"-i rədd edir, biz credential-lı sorğu ümumiyyətlə
// istifadə etmirik).
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
		}
		originSet[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowedOrigins) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || originSet[origin]) {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					// Vary: Origin — proxy/CDN-lərin fərqli origin-lər üçün
					// yanlışlıqla eyni cache-lənmiş cavabı qaytarmasının qarşısını alır.
					w.Header().Set("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
				w.Header().Set("Access-Control-Max-Age", "600")
			}

			// Preflight sorğusu — brauzer əsl sorğudan əvvəl OPTIONS göndərir,
			// bunu auth/rate-limit-ə çatdırmadan birbaşa cavablandırırıq.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
