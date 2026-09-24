package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"keychain/internal/vault"
)

type passwordReq struct {
	Password string `json:"password"`
}

type changeReq struct {
	Old string `json:"old"`
	New string `json:"new"`
}

type entryReq struct {
	Title    string   `json:"title"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	URL      string   `json:"url"`
	Notes    string   `json:"notes"`
	Tags     []string `json:"tags"`
}

func (e entryReq) toEntry() vault.Entry {
	return vault.Entry{
		Title: e.Title, Username: e.Username, Password: e.Password,
		URL: e.URL, Notes: e.Notes, Tags: e.Tags,
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"initialized": vault.Exists(s.cfg.Path),
		"unlocked":    s.session(r, false) != nil,
	})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req passwordReq
	if !decode(w, r, &req) {
		return
	}
	if len([]rune(req.Password)) < minPassword {
		writeError(w, http.StatusBadRequest, "主密碼至少需 8 個字元")
		return
	}
	s.unlockMu.Lock()
	defer s.unlockMu.Unlock()
	v, err := vault.Create(s.cfg.Path, req.Password)
	if errors.Is(err, vault.ErrExists) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "建立保險庫失敗")
		return
	}
	s.startSession(w, v)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	var req passwordReq
	if !decode(w, r, &req) {
		return
	}
	s.unlockMu.Lock()
	defer s.unlockMu.Unlock()
	if !vault.Exists(s.cfg.Path) {
		writeError(w, http.StatusConflict, "尚未建立保險庫")
		return
	}
	v, err := vault.Open(s.cfg.Path, req.Password)
	if errors.Is(err, vault.ErrBadPassword) {
		time.Sleep(s.cfg.FailDelay)
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "開啟保險庫失敗")
		return
	}
	s.startSession(w, v)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	if s.session(r, false) != nil {
		s.Lock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/api", MaxAge: -1, Secure: s.cfg.SecureCookie})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request, v *vault.Vault) {
	writeJSON(w, http.StatusOK, v.List(r.URL.Query().Get("q")))
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, v *vault.Vault) {
	e, err := v.Get(r.PathValue("id"))
	if err != nil {
		writeVaultError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request, v *vault.Vault) {
	var req entryReq
	if !decode(w, r, &req) {
		return
	}
	e, err := v.Add(req.toEntry())
	if err != nil {
		writeVaultError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request, v *vault.Vault) {
	var req entryReq
	if !decode(w, r, &req) {
		return
	}
	e, err := v.Update(r.PathValue("id"), req.toEntry())
	if err != nil {
		writeVaultError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, v *vault.Vault) {
	if err := v.Delete(r.PathValue("id")); err != nil {
		writeVaultError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, v *vault.Vault) {
	var req changeReq
	if !decode(w, r, &req) {
		return
	}
	if len([]rune(req.New)) < minPassword {
		writeError(w, http.StatusBadRequest, "主密碼至少需 8 個字元")
		return
	}
	err := v.ChangePassword(req.Old, req.New)
	if errors.Is(err, vault.ErrBadPassword) {
		time.Sleep(s.cfg.FailDelay)
		writeError(w, http.StatusBadRequest, "目前主密碼錯誤")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "更換主密碼失敗")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type restoreReq struct {
	Password string            `json:"password"`
	Mode     vault.RestoreMode `json:"mode"`
	Backup   json.RawMessage   `json:"backup"`
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request, v *vault.Vault) {
	var req passwordReq
	if !decode(w, r, &req) {
		return
	}
	if len([]rune(req.Password)) < minPassword {
		writeError(w, http.StatusBadRequest, "備份密碼至少需 8 個字元")
		return
	}
	data, err := v.Backup(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "產生備份失敗")
		return
	}
	name := "keychain-backup-" + time.Now().Format("20060102-150405") + ".kcbak"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Write(data)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request, v *vault.Vault) {
	var req restoreReq
	if !decodeLimit(w, r, &req, maxRestore) {
		return
	}
	if req.Mode != vault.RestoreMerge && req.Mode != vault.RestoreReplace {
		writeError(w, http.StatusBadRequest, "還原方式須為 merge 或 replace")
		return
	}
	entries, err := vault.ReadBackup(req.Backup, req.Password)
	if errors.Is(err, vault.ErrBadBackup) {
		time.Sleep(s.cfg.FailDelay)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := v.Restore(entries, req.Mode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "還原失敗")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeLimit(w, r, dst, maxBody)
}

func decodeLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "需使用 application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "請求格式錯誤")
		return false
	}
	return true
}

func writeVaultError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vault.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, vault.ErrTitleMissing):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "儲存失敗")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
