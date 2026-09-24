package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"keychain/internal/server"
	"keychain/internal/web"
)

func main() {
	home, _ := os.UserHomeDir()
	addr := flag.String("addr", "127.0.0.1:8787", "監聽位址")
	path := flag.String("data", filepath.Join(home, ".keychain", "vault.json"), "保險庫檔案路徑")
	idle := flag.Duration("idle", 10*time.Minute, "閒置自動上鎖時間")
	flag.Parse()

	srv := server.New(server.Config{Path: *path, IdleTimeout: *idle, FailDelay: time.Second, Static: web.FS()})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go srv.RunJanitor(ctx, 30*time.Second)

	hs := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()

	log.Printf("keychain 啟動於 http://%s（資料: %s）", *addr, *path)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	srv.Lock()
}
