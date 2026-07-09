package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimit — IP başına dəqiqədə `perMinute` sorğu limiti tətbiq edən token
// bucket (vedrə) alqoritmi. `burst` qısa müddətli "partlayışlara" (məs. bir
// səhifə açılışında paralel bir neçə sorğu) icazə verir.
//
// NİYƏ LAZIMDIR: API_KEY-lə qorunan endpoint-lər belə (POST /sources kimi)
// key sızsa/təxmin edilsə, məhdudiyyətsiz sürətdə çağırıla bilər. Bu, DB-ni
// yükləyə (çoxlu INSERT) və ya sadəcə resurs tükənməsinə səbəb ola bilər.
// Rate-limit, autentifikasiyadan ASILI OLMAYAN əlavə bir müdafiə qatıdır.
//
// XARİCİ ASILILIQ YOXDUR — golang.org/x/time/rate əvəzinə sadə, dependency-siz
// bir tətbiq seçildi ki, go.mod-a yeni modul əlavə olunması lazım gəlməsin.
func RateLimit(perMinute, burst int) func(http.Handler) http.Handler {
	l := newLimiter(perMinute, burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)

			if !l.allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"çox sayda sorğu, bir az sonra yenidən cəhd edin"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIP — request-in RemoteAddr-ından (host:port) yalnız host hissəsini
// çıxarır. QƏSDƏN X-Forwarded-For/X-Real-IP kimi header-lərə güvənmirik —
// bunlar client tərəfindən sərbəst şəkildə saxtalaşdırıla bilər (spoofable);
// yalnız reverse-proxy-nin ÖZÜ etibarlı şəkildə təyin etdiyi mühitlərdə
// istifadə edilməlidirlər. Tətbiq birbaşa (proxy-siz) işə salındığı üçün
// RemoteAddr etibarlı yeganə mənbədir.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// bucket — tək bir IP üçün token bucket vəziyyəti.
type bucket struct {
	mu       sync.Mutex
	tokens   float64
	lastSeen time.Time
}

// limiter — bütün IP-lər üçün bucket-ləri saxlayır və vaxtaşırı istifadə
// olunmayanları təmizləyir (yaddaş sızmasının qarşısını almaq üçün — əks
// halda hər unikal IP üçün əbədi bir bucket yaddaşda qalardı).
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // saniyədə neçə token "dolur"
	burst   float64 // maksimum tutum
}

func newLimiter(perMinute, burst int) *limiter {
	if burst <= 0 {
		burst = perMinute
	}
	if burst <= 0 {
		burst = 1
	}

	l := &limiter{
		buckets: make(map[string]*bucket),
		rate:    float64(perMinute) / 60.0,
		burst:   float64(burst),
	}

	go l.cleanupLoop()

	return l
}

// cleanupLoop — 10 dəqiqədən bir, son 10 dəqiqədə görünməyən IP-lərin
// bucket-lərini silir. Server aylarla işlədiyi üçün (uzunmüddətli poller),
// bu olmasa map sonsuz böyüyərdi.
func (l *limiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		l.mu.Lock()
		for ip, b := range l.buckets {
			b.mu.Lock()
			stale := b.lastSeen.Before(cutoff)
			b.mu.Unlock()
			if stale {
				delete(l.buckets, ip)
			}
		}
		l.mu.Unlock()
	}
}

func (l *limiter) getBucket(ip string) *bucket {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: l.burst, lastSeen: time.Now()}
		l.buckets[ip] = b
	}
	return b
}

// allow — bu IP üçün bir sorğuya icazə verilib-verilmədiyini qaytarır və
// icazə verilirsə bir token istehlak edir. Vaxt keçdikcə token-lər
// `rate`-ə uyğun tədricən "yenidən dolur" (`burst`-dən çox yığılmır).
func (l *limiter) allow(ip string) bool {
	b := l.getBucket(ip)

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.lastSeen = now

	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}

	if b.tokens < 1 {
		return false
	}

	b.tokens--
	return true
}
