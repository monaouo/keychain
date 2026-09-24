package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"keychain/internal/vault"
)

const (
	cookieName  = "keychain_session"
	csrfHeader  = "X-Keychain"
	minPassword = 8
	maxBody     = 1 << 20
)

// Config 為伺服器設定。
type Config struct {
	Path        string        // 保險庫路徑
	IdleTimeout time.Duration // 閒置上鎖
	FailDelay   time.Duration // 失敗延遲
}

// Server 管理保險庫與工作階段。
type Server struct {
	cfg Config
	now func() time.Time

	mu       sync.Mutex
	vault    *vault.Vault
	token    string
	lastSeen time.Time
	unlockMu sync.Mutex // 序列化解鎖
}

// New 建立伺服器。
func New(cfg Config) *Server {
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 10 * time.Minute
	}
	return &Server{cfg: cfg, now: time.Now}
}

// Handler 回傳 API 路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/unlock", s.handleUnlock)
	mux.HandleFunc("POST /api/lock", s.handleLock)
	mux.Handle("GET /api/entries", s.auth(s.handleList))
	mux.Handle("POST /api/entries", s.auth(s.handleCreate))
	mux.Handle("GET /api/entries/{id}", s.auth(s.handleGet))
	mux.Handle("PUT /api/entries/{id}", s.auth(s.handleUpdate))
	mux.Handle("DELETE /api/entries/{id}", s.auth(s.handleDelete))
	mux.Handle("POST /api/password", s.auth(s.handleChangePassword))
	return securityHeaders(csrfGuard(mux))
}

// RunJanitor 定期檢查閒置並上鎖。
func (s *Server) RunJanitor(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Lock()
			return
		case <-t.C:
			s.mu.Lock()
			s.expireLocked()
			s.mu.Unlock()
		}
	}
}

// Lock 上鎖並清除金鑰。
func (s *Server) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockLocked()
}

func (s *Server) lockLocked() {
	if s.vault != nil {
		s.vault.Close()
	}
	s.vault, s.token = nil, ""
}

func (s *Server) expireLocked() {
	if s.vault != nil && s.now().Sub(s.lastSeen) > s.cfg.IdleTimeout {
		s.lockLocked()
	}
}

// startSession 設定新保險庫並發 token。
func (s *Server) startSession(w http.ResponseWriter, v *vault.Vault) {
	b := make([]byte, 32)
	rand.Read(b)
	s.mu.Lock()
	s.lockLocked()
	s.vault, s.token, s.lastSeen = v, hex.EncodeToString(b), s.now()
	token := s.token
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/api",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

// session 驗證 cookie 並取得保險庫。
func (s *Server) session(r *http.Request) *vault.Vault {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	if s.vault == nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.token)) != 1 {
		return nil
	}
	s.lastSeen = s.now()
	return s.vault
}

type authed func(w http.ResponseWriter, r *http.Request, v *vault.Vault)

func (s *Server) auth(h authed) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := s.session(r)
		if v == nil {
			writeError(w, http.StatusUnauthorized, "保險庫已上鎖")
			return
		}
		h(w, r, v)
	})
}

// csrfGuard 要求變更請求帶自訂標頭。
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(csrfHeader) != "1" {
			writeError(w, http.StatusForbidden, "缺少 "+csrfHeader+" 標頭")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
