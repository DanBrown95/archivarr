package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
	"golang.org/x/crypto/bcrypt"
)

const (
	// sessionCookie is the name of the session cookie.
	sessionCookie = "archivarr_session"
	// sessionTTL is how long a session lasts; it slides forward on each use.
	sessionTTL = 30 * 24 * time.Hour
	// bcryptCost is the bcrypt work factor for password hashing.
	bcryptCost = 12
	// minPasswordLen is the shortest password we accept.
	minPasswordLen = 8
	// maxUsernameLen bounds the username to keep things sane.
	maxUsernameLen = 64
	// settingAPIKey is the settings-table key holding the automation API key.
	settingAPIKey = "api.key"
	// apiKeyHeader is the header headless clients use to authenticate.
	apiKeyHeader = "X-Api-Key"
)

// ctxKeyUser is the context key under which the authenticated user is stored.
type ctxKeyUser struct{}

// currentUser returns the authenticated user from the request context, if any.
func currentUser(r *http.Request) (db.User, bool) {
	u, ok := r.Context().Value(ctxKeyUser{}).(db.User)
	return u, ok
}

// --- credential helpers ---

func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(b), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// newSessionToken returns a 256-bit URL-safe random token.
func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// newAPIKey returns a 128-bit hex API key (32 chars), matching *arr conventions.
func newAPIKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// extractAPIKey pulls an API key from the request: the X-Api-Key header or an
// `Authorization: Bearer <key>` header. The `?apikey=` query param is
// deliberately unsupported so keys don't end up in request logs.
func extractAPIKey(r *http.Request) string {
	if k := r.Header.Get(apiKeyHeader); k != "" {
		return k
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

// ensureAPIKey returns the stored API key, generating and persisting one the
// first time it is requested.
func (s *server) ensureAPIKey(ctx context.Context) (string, error) {
	v, ok, err := s.db.GetSetting(ctx, settingAPIKey)
	if err != nil {
		return "", err
	}
	if ok && v != "" {
		return v, nil
	}
	key, err := newAPIKey()
	if err != nil {
		return "", err
	}
	if err := s.db.SetSetting(ctx, settingAPIKey, key); err != nil {
		return "", err
	}
	return key, nil
}

// validateCredentials applies basic username/password rules.
func validateCredentials(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if len(username) > maxUsernameLen {
		return errors.New("username is too long")
	}
	if len(password) < minPasswordLen {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}

// --- cookie helpers ---

// isHTTPS reports whether the request reached us over TLS (directly or via a
// trusted reverse proxy that set X-Forwarded-Proto).
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// startSession mints a session for a user and sets the cookie.
func (s *server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token, err := newSessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.db.CreateSession(r.Context(), token, userID, expires.Unix()); err != nil {
		return err
	}
	s.setSessionCookie(w, r, token, expires)
	return nil
}

// --- handlers ---

// authStatus reports whether first-run setup is needed and whether the caller
// is currently authenticated. It is public (no session required).
func (s *server) authStatus(w http.ResponseWriter, r *http.Request) {
	count, err := s.db.UserCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{
		"setupRequired": count == 0,
		"authenticated": false,
	}
	if u, ok := s.userFromCookie(r); ok {
		resp["authenticated"] = true
		resp["username"] = u.Username
	}
	writeJSON(w, http.StatusOK, resp)
}

// setupRequest / loginRequest share the same shape.
type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// setup creates the first admin user. It only works while no user exists, so it
// cannot be used to add accounts once the app is initialized.
func (s *server) setup(w http.ResponseWriter, r *http.Request) {
	count, err := s.db.UserCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "setup has already been completed")
		return
	}

	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := validateCredentials(req.Username, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	u, err := s.db.CreateUser(r.Context(), req.Username, hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"username": u.Username})
}

// login authenticates a user and starts a session. Failed attempts are
// throttled per client IP.
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if retryAfter, ok := s.loginLimiter.blocked(ip); ok {
		w.Header().Set("Retry-After", retryAfter)
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	u, err := s.db.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
	if err != nil || !checkPassword(u.PasswordHash, req.Password) {
		s.loginLimiter.fail(ip)
		// Generic message: don't reveal whether the username exists.
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	s.loginLimiter.reset(ip)
	if err := s.startSession(w, r, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	_ = s.db.DeleteExpiredSessions(r.Context()) // opportunistic cleanup
	writeJSON(w, http.StatusOK, map[string]any{"username": u.Username})
}

// logout deletes the current session and clears the cookie.
func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.db.DeleteSession(r.Context(), c.Value)
	}
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// accountRequest changes the current user's username and/or password. The
// current password is always required to confirm the change.
type accountRequest struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *server) updateAccount(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req accountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !checkPassword(u.PasswordHash, req.CurrentPassword) {
		// 403, not 401: the session is valid, only the re-auth check failed.
		// A 401 here would be mistaken for an expired session and log the user out.
		writeError(w, http.StatusForbidden, "current password is incorrect")
		return
	}

	newUsername := strings.TrimSpace(req.Username)
	if newUsername == "" {
		newUsername = u.Username
	}
	// The new password is optional (username-only change keeps the old one).
	newHash := u.PasswordHash
	if req.NewPassword != "" {
		if err := validateCredentials(newUsername, req.NewPassword); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h, err := hashPassword(req.NewPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not hash password")
			return
		}
		newHash = h
	} else if len(newUsername) > maxUsernameLen {
		writeError(w, http.StatusBadRequest, "username is too long")
		return
	}

	if err := s.db.UpdateUserCredentials(r.Context(), u.ID, newUsername, newHash); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate every session (including this one), then re-issue a fresh
	// session so the current browser stays logged in but old cookies die.
	if err := s.db.DeleteUserSessions(r.Context(), u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not refresh session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": newUsername})
}

// --- middleware ---

// userFromCookie resolves the request's session cookie to a user, sliding the
// session expiry forward. It does not write any error response.
func (s *server) userFromCookie(r *http.Request) (db.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return db.User{}, false
	}
	sess, err := s.db.GetSession(r.Context(), c.Value)
	if err != nil {
		return db.User{}, false
	}
	u, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		return db.User{}, false
	}
	return u, true
}

// authenticateSession validates the session cookie, slides its expiry forward,
// and returns the user. It clears a stale cookie but writes no error response.
func (s *server) authenticateSession(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return db.User{}, false
	}
	sess, err := s.db.GetSession(r.Context(), c.Value)
	if err != nil {
		s.clearSessionCookie(w, r)
		return db.User{}, false
	}
	u, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		return db.User{}, false
	}
	// Slide the session forward so active users stay logged in.
	expires := time.Now().Add(sessionTTL)
	_ = s.db.TouchSession(r.Context(), sess.Token, expires.Unix())
	s.setSessionCookie(w, r, sess.Token, expires)
	return u, true
}

// requireAuth is middleware that requires a valid browser session, rejecting
// other requests with 401 and storing the user in the request context. Used for
// account/session management, which the API key intentionally cannot perform.
func (s *server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.authenticateSession(w, r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUser{}, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAuthOrKey is middleware for the data API: it accepts either a valid
// browser session or a valid X-Api-Key / Bearer API key. A key grants full data
// access but carries no user (so account-management handlers stay session-only).
func (s *server) requireAuthOrKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if provided := extractAPIKey(r); provided != "" {
			stored, _, err := s.db.GetSetting(r.Context(), settingAPIKey)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if stored != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(stored)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid API key")
			return
		}
		u, ok := s.authenticateSession(w, r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUser{}, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getAPIKey returns the automation API key, generating one on first request.
func (s *server) getAPIKey(w http.ResponseWriter, r *http.Request) {
	key, err := s.ensureAPIKey(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"apiKey": key})
}

// regenerateAPIKey rotates the API key, invalidating any existing integrations.
func (s *server) regenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	key, err := newAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate key")
		return
	}
	if err := s.db.SetSetting(r.Context(), settingAPIKey, key); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"apiKey": key})
}

// --- login throttle ---

const (
	loginMaxFailures = 5
	loginWindow      = 15 * time.Minute
	loginBlock       = 15 * time.Minute
)

// loginLimiter is a tiny in-memory per-IP failed-login throttle. It is not a
// substitute for an edge rate limiter, just a cheap brute-force speed bump.
type loginLimiter struct {
	mu      sync.Mutex
	clock   func() time.Time
	entries map[string]*limiterEntry
}

type limiterEntry struct {
	failures   int
	windowEnds time.Time
	blockUntil time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{clock: time.Now, entries: make(map[string]*limiterEntry)}
}

// blocked reports whether the IP is currently locked out, and if so a
// Retry-After value in seconds.
func (l *loginLimiter) blocked(ip string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[ip]
	if e == nil {
		return "", false
	}
	now := l.clock()
	if now.Before(e.blockUntil) {
		secs := int(e.blockUntil.Sub(now).Seconds()) + 1
		return strconv.Itoa(secs), true
	}
	return "", false
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	e := l.entries[ip]
	if e == nil || now.After(e.windowEnds) {
		e = &limiterEntry{windowEnds: now.Add(loginWindow)}
		l.entries[ip] = e
	}
	e.failures++
	if e.failures >= loginMaxFailures {
		e.blockUntil = now.Add(loginBlock)
		e.failures = 0
		e.windowEnds = now.Add(loginBlock)
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

// clientIP extracts the best-effort client IP for throttling. chi's RealIP
// middleware has already normalized RemoteAddr from forwarded headers.
func clientIP(r *http.Request) string {
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}
