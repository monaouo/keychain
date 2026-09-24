package config

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config 為執行設定。
type Config struct {
	Addr         string
	Data         string
	Idle         time.Duration
	SecureCookie bool
}

// Load 依序套用預設、環境變數、參數。
func Load(name string, args []string, getenv func(string) string, out io.Writer) (Config, error) {
	home, _ := os.UserHomeDir()
	c := Config{
		Addr: "127.0.0.1:8787",
		Data: filepath.Join(home, ".keychain", "vault.json"),
		Idle: 10 * time.Minute,
	}
	if v := getenv("KEYCHAIN_ADDR"); v != "" {
		c.Addr = v
	}
	if v := getenv("KEYCHAIN_DATA"); v != "" {
		c.Data = v
	}
	if v := getenv("KEYCHAIN_IDLE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("KEYCHAIN_IDLE 格式錯誤: %w", err)
		}
		c.Idle = d
	}
	if v := getenv("KEYCHAIN_SECURE_COOKIE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("KEYCHAIN_SECURE_COOKIE 格式錯誤: %w", err)
		}
		c.SecureCookie = b
	}

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&c.Addr, "addr", c.Addr, "監聽位址（KEYCHAIN_ADDR）")
	fs.StringVar(&c.Data, "data", c.Data, "保險庫檔案路徑（KEYCHAIN_DATA）")
	fs.DurationVar(&c.Idle, "idle", c.Idle, "閒置自動上鎖時間（KEYCHAIN_IDLE）")
	fs.BoolVar(&c.SecureCookie, "secure-cookie", c.SecureCookie, "cookie 加上 Secure，HTTPS 代理時使用（KEYCHAIN_SECURE_COOKIE）")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if c.Idle <= 0 {
		return c, fmt.Errorf("閒置時間必須大於 0")
	}
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		return c, fmt.Errorf("監聽位址格式錯誤: %w", err)
	}
	return c, nil
}

// HealthURL 回傳健康檢查網址。
func (c Config) HealthURL() string {
	host, port, _ := net.SplitHostPort(c.Addr)
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/api/status"
}
