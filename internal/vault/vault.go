package vault

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Entry 為一筆帳號資料。
type Entry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	URL       string    `json:"url"`
	Notes     string    `json:"notes"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

var (
	ErrNotFound     = errors.New("找不到該筆資料")
	ErrExists       = errors.New("保險庫已存在")
	ErrTitleMissing = errors.New("標題不可為空")
)

type fileFormat struct {
	Version int    `json:"version"`
	Salt    []byte `json:"salt"`
	Iter    int    `json:"iter"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"`
}

// Vault 為已解鎖的保險庫。
type Vault struct {
	mu      sync.RWMutex
	path    string
	key     []byte
	salt    []byte
	iter    int
	entries []Entry
}

// Exists 回報保險庫檔案是否存在。
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Create 建立新的空保險庫。
func Create(path, password string) (*Vault, error) {
	if Exists(path) {
		return nil, ErrExists
	}
	salt := randomBytes(saltLen)
	key, err := deriveKey(password, salt, Iterations)
	if err != nil {
		return nil, err
	}
	v := &Vault{path: path, key: key, salt: salt, iter: Iterations, entries: []Entry{}}
	if err := v.save(); err != nil {
		return nil, err
	}
	return v, nil
}

// Open 以主密碼解鎖保險庫。
func Open(path, password string) (*Vault, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("檔案格式錯誤: %w", err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("不支援的版本: %d", f.Version)
	}
	key, err := deriveKey(password, f.Salt, f.Iter)
	if err != nil {
		return nil, err
	}
	plain, err := open(key, f.Nonce, f.Data, vaultAAD)
	if err != nil {
		wipe(key)
		return nil, err
	}
	defer wipe(plain)
	var entries []Entry
	if err := json.Unmarshal(plain, &entries); err != nil {
		wipe(key)
		return nil, fmt.Errorf("資料解析失敗: %w", err)
	}
	if entries == nil {
		entries = []Entry{}
	}
	return &Vault{path: path, key: key, salt: f.Salt, iter: f.Iter, entries: entries}, nil
}

// Close 清除記憶體中的金鑰。
func (v *Vault) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	wipe(v.key)
	v.key = nil
	v.entries = nil
}

// List 依關鍵字篩選並按標題排序。
func (v *Vault) List(query string) []Entry {
	v.mu.RLock()
	defer v.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Entry, 0, len(v.entries))
	for _, e := range v.entries {
		if q == "" || e.matches(q) {
			out = append(out, e.clone())
		}
	}
	slices.SortFunc(out, func(a, b Entry) int {
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})
	return out
}

// Get 取得單筆資料。
func (v *Vault) Get(id string) (Entry, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	i := v.index(id)
	if i < 0 {
		return Entry{}, ErrNotFound
	}
	return v.entries[i].clone(), nil
}

// Add 新增一筆資料。
func (v *Vault) Add(e Entry) (Entry, error) {
	if err := e.normalize(); err != nil {
		return Entry{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	now := time.Now().UTC()
	e.ID = hex.EncodeToString(randomBytes(16))
	e.CreatedAt, e.UpdatedAt = now, now
	v.entries = append(v.entries, e)
	if err := v.save(); err != nil {
		v.entries = v.entries[:len(v.entries)-1]
		return Entry{}, err
	}
	return e.clone(), nil
}

// Update 覆寫指定資料。
func (v *Vault) Update(id string, e Entry) (Entry, error) {
	if err := e.normalize(); err != nil {
		return Entry{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	i := v.index(id)
	if i < 0 {
		return Entry{}, ErrNotFound
	}
	old := v.entries[i]
	e.ID, e.CreatedAt, e.UpdatedAt = old.ID, old.CreatedAt, time.Now().UTC()
	v.entries[i] = e
	if err := v.save(); err != nil {
		v.entries[i] = old
		return Entry{}, err
	}
	return e.clone(), nil
}

// Delete 刪除指定資料。
func (v *Vault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	i := v.index(id)
	if i < 0 {
		return ErrNotFound
	}
	prev := v.entries
	v.entries = slices.Delete(slices.Clone(prev), i, i+1)
	if err := v.save(); err != nil {
		v.entries = prev
		return err
	}
	return nil
}

// ChangePassword 更換主密碼並重新加密。
func (v *Vault) ChangePassword(oldPw, newPw string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	check, err := deriveKey(oldPw, v.salt, v.iter)
	if err != nil {
		return err
	}
	same := subtle.ConstantTimeCompare(check, v.key) == 1
	wipe(check)
	if !same {
		return ErrBadPassword
	}
	salt := randomBytes(saltLen)
	key, err := deriveKey(newPw, salt, Iterations)
	if err != nil {
		return err
	}
	oldKey, oldSalt, oldIter := v.key, v.salt, v.iter
	v.key, v.salt, v.iter = key, salt, Iterations
	if err := v.save(); err != nil {
		wipe(key)
		v.key, v.salt, v.iter = oldKey, oldSalt, oldIter
		return err
	}
	wipe(oldKey)
	return nil
}

func (v *Vault) index(id string) int {
	return slices.IndexFunc(v.entries, func(e Entry) bool { return e.ID == id })
}

// save 以原子方式寫入檔案。
func (v *Vault) save() error {
	plain, err := json.Marshal(v.entries)
	if err != nil {
		return err
	}
	defer wipe(plain)
	nonce, ct, err := seal(v.key, plain, vaultAAD)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(fileFormat{
		Version: 1, Salt: v.salt, Iter: v.iter, Nonce: nonce, Data: ct,
	}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(v.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".vault-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), v.path)
}

func (e *Entry) normalize() error {
	e.Title = strings.TrimSpace(e.Title)
	if e.Title == "" {
		return ErrTitleMissing
	}
	e.Username = strings.TrimSpace(e.Username)
	e.URL = strings.TrimSpace(e.URL)
	tags := make([]string, 0, len(e.Tags))
	for _, t := range e.Tags {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	e.Tags = tags
	return nil
}

func (e Entry) matches(q string) bool {
	fields := append([]string{e.Title, e.Username, e.URL, e.Notes}, e.Tags...)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

func (e Entry) clone() Entry {
	e.Tags = slices.Clone(e.Tags)
	if e.Tags == nil {
		e.Tags = []string{}
	}
	return e
}
