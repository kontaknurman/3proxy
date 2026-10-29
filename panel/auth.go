package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	sessionCookie = "panel_session"
	sessionTTL    = 12 * time.Hour
	maxFails      = 5
	lockTime      = 10 * time.Minute
)

type session struct {
	expires time.Time
	flash   string
	isError bool
}

// auth keeps the sessions in memory: restarting the panel logs everybody
// out.
type auth struct {
	password [32]byte
	secure   bool

	mu       sync.Mutex
	sessions map[string]*session
	fails    map[string]*failure
}

type failure struct {
	count int
	until time.Time
}

func newAuth(password string, secure bool) *auth {
	return &auth{
		password: sha256.Sum256([]byte(password)),
		secure:   secure,
		sessions: map[string]*session{},
		fails:    map[string]*failure{},
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// locked tells whether the client failed to log in too many times.
func (a *auth) locked(r *http.Request) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.fails[clientIP(r)]
	return f != nil && f.count >= maxFails && time.Now().Before(f.until)
}

// login checks the password and starts a session on success.
func (a *auth) login(w http.ResponseWriter, r *http.Request, password string) bool {
	sum := sha256.Sum256([]byte(password))
	ok := subtle.ConstantTimeCompare(sum[:], a.password[:]) == 1
	ip := clientIP(r)

	a.mu.Lock()
	defer a.mu.Unlock()
	if !ok {
		f := a.fails[ip]
		if f == nil || time.Now().After(f.until) {
			f = &failure{}
			a.fails[ip] = f
		}
		f.count++
		f.until = time.Now().Add(lockTime)
		return false
	}
	delete(a.fails, ip)

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token := hex.EncodeToString(b)
	now := time.Now()
	for t, s := range a.sessions {
		if now.After(s.expires) {
			delete(a.sessions, t)
		}
	}
	a.sessions[token] = &session{expires: now.Add(sessionTTL)}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteStrictMode,
	})
	return true
}

func (a *auth) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
}

// session returns the session of the request, nil if there is none. The
// caller must hold a.mu.
func (a *auth) session(r *http.Request) *session {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	s := a.sessions[c.Value]
	if s == nil || time.Now().After(s.expires) {
		return nil
	}
	return s
}

func (a *auth) loggedIn(r *http.Request) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session(r) != nil
}

// setFlash stores a message shown on the next page.
func (a *auth) setFlash(r *http.Request, msg string, isError bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s := a.session(r); s != nil {
		s.flash, s.isError = msg, isError
	}
}

func (a *auth) takeFlash(r *http.Request) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.session(r)
	if s == nil {
		return "", false
	}
	msg, isError := s.flash, s.isError
	s.flash, s.isError = "", false
	return msg, isError
}

// sameOrigin rejects requests a browser sent on behalf of another site,
// another port of the VPS included. The session cookie is SameSite=Strict
// already; this covers older browsers. Requests without these headers,
// from curl for instance, pass.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return false
	}
	// Browsers send "null" when the referrer policy hides the origin, which
	// securityHeaders avoids for this site's own requests.
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}
