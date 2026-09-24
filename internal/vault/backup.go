package vault

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

const backupFormat = "keychain-backup"

var (
	ErrBadBackup     = errors.New("備份密碼錯誤或檔案已損毀")
	ErrInvalidBackup = errors.New("不是有效的 keychain 備份檔")
)

// RestoreMode 為還原方式。
type RestoreMode string

const (
	RestoreMerge   RestoreMode = "merge"   // 合併
	RestoreReplace RestoreMode = "replace" // 覆蓋
)

// RestoreResult 為還原統計。
type RestoreResult struct {
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Removed   int `json:"removed"`
	Total     int `json:"total"`
}

type backupFile struct {
	Format    string    `json:"format"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	KDF       string    `json:"kdf"`
	Iter      int       `json:"iter"`
	Salt      []byte    `json:"salt"`
	Nonce     []byte    `json:"nonce"`
	Data      []byte    `json:"data"`
}

type backupPayload struct {
	Entries []Entry `json:"entries"`
}

// Backup 以備份密碼加密匯出。
func (v *Vault) Backup(password string) ([]byte, error) {
	v.mu.RLock()
	plain, err := json.Marshal(backupPayload{Entries: v.entries})
	v.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	defer wipe(plain)

	salt := randomBytes(saltLen)
	key, err := deriveKey(password, salt, Iterations)
	if err != nil {
		return nil, err
	}
	defer wipe(key)
	nonce, ct, err := seal(key, plain, backupAAD)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(backupFile{
		Format: backupFormat, Version: 1, CreatedAt: time.Now().UTC(),
		KDF: "pbkdf2-sha256", Iter: Iterations, Salt: salt, Nonce: nonce, Data: ct,
	}, "", "  ")
}

// ReadBackup 解密備份檔。
func ReadBackup(raw []byte, password string) ([]Entry, error) {
	var f backupFile
	if err := json.Unmarshal(raw, &f); err != nil || f.Format != backupFormat {
		return nil, ErrInvalidBackup
	}
	if f.Version != 1 || f.KDF != "pbkdf2-sha256" {
		return nil, fmt.Errorf("不支援的備份版本: %d", f.Version)
	}
	if f.Iter < 1 || len(f.Salt) == 0 || len(f.Nonce) == 0 {
		return nil, ErrInvalidBackup
	}
	key, err := deriveKey(password, f.Salt, f.Iter)
	if err != nil {
		return nil, err
	}
	defer wipe(key)
	plain, err := open(key, f.Nonce, f.Data, backupAAD)
	if err != nil {
		return nil, ErrBadBackup
	}
	defer wipe(plain)
	var p backupPayload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, ErrInvalidBackup
	}
	for i := range p.Entries {
		if err := p.Entries[i].normalize(); err != nil {
			return nil, fmt.Errorf("第 %d 筆資料無效: %w", i+1, err)
		}
	}
	return p.Entries, nil
}

// Restore 合併或覆蓋還原。
func (v *Vault) Restore(entries []Entry, mode RestoreMode) (RestoreResult, error) {
	if mode != RestoreMerge && mode != RestoreReplace {
		return RestoreResult{}, fmt.Errorf("未知的還原方式: %q", mode)
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	var res RestoreResult
	var next []Entry
	if mode == RestoreReplace {
		res.Removed = len(v.entries)
	} else {
		next = slices.Clone(v.entries)
	}
	now := time.Now().UTC()
	for _, e := range entries {
		e = e.clone()
		if e.ID == "" {
			e.ID = hex.EncodeToString(randomBytes(16))
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
		if e.UpdatedAt.IsZero() {
			e.UpdatedAt = e.CreatedAt
		}
		i := slices.IndexFunc(next, func(x Entry) bool { return x.ID == e.ID })
		switch {
		case i < 0:
			next = append(next, e)
			res.Added++
		case e.UpdatedAt.After(next[i].UpdatedAt):
			next[i] = e
			res.Updated++
		default:
			res.Unchanged++
		}
	}
	if next == nil {
		next = []Entry{}
	}
	prev := v.entries
	v.entries = next
	if err := v.save(); err != nil {
		v.entries = prev
		return RestoreResult{}, err
	}
	res.Total = len(next)
	return res, nil
}
