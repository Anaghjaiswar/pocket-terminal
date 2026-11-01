package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

const (
	// DefaultTokenBytes is the number of random bytes used to generate a 16-char hex token.
	DefaultTokenBytes = 8
	// SessionCookieName stores the verified token so subsequent static asset requests succeed seamlessly.
	SessionCookieName = "pt_token"
)

// GenerateToken generates a cryptographically secure 16-character hexadecimal session token.
func GenerateToken() (string, error) {
	buf := make([]byte, DefaultTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ValidateRequest checks if the incoming HTTP request carries a valid session token
// via the ?token= query parameter, Authorization Bearer header, X-Auth-Token header,
// or the pt_token session cookie.
func ValidateRequest(r *http.Request, expectedToken string) bool {
	if expectedToken == "" {
		return false
	}

	candidates := []string{
		strings.TrimSpace(r.URL.Query().Get("token")),
		strings.TrimSpace(r.Header.Get("X-Auth-Token")),
	}

	if authHeader := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		candidates = append(candidates, strings.TrimSpace(authHeader[7:]))
	}

	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		candidates = append(candidates, strings.TrimSpace(cookie.Value))
	}

	expectedBytes := []byte(expectedToken)
	for _, candidate := range candidates {
		if len(candidate) == len(expectedToken) && subtle.ConstantTimeCompare([]byte(candidate), expectedBytes) == 1 {
			return true
		}
	}
	return false
}

// Middleware enforces token authentication before allowing access to protected HTTP/WebSocket handlers.
// When a valid ?token= query parameter is supplied on the initial page load, it also sets an HttpOnly
// session cookie so static assets (/style.css, /app.js) load without requiring query strings on every asset URL.
func Middleware(expectedToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ValidateRequest(r, expectedToken) {
			http.Error(w, "401 Unauthorized: valid ?token= parameter required", http.StatusUnauthorized)
			return
		}

		if qToken := strings.TrimSpace(r.URL.Query().Get("token")); qToken != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     SessionCookieName,
				Value:    expectedToken,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		next.ServeHTTP(w, r)
	})
}
