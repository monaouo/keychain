package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"keychain/internal/config"
	"keychain/internal/server"
	"keychain/internal/web"
)

func main() {
	args := os.Args[1:]
	health := len(args) > 0 && args[0] == "healthcheck"
	if health {
		args = args[1:]
	}
	cfg, err := config.Load("keychain", args, os.Getenv, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if health {
		os.Exit(healthcheck(cfg))
	}
	serve(cfg)
}

// healthcheck 供容器探測。
func healthcheck(cfg config.Config) int {
	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Get(cfg.HealthURL())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "狀態碼", resp.StatusCode)
		return 1
	}
	return 0
}

func serve(cfg config.Config) {
	srv := server.New(server.Config{
		Path:         cfg.Data,
		IdleTimeout:  cfg.Idle,
		FailDelay:    time.Second,
		SecureCookie: cfg.SecureCookie,
		Static:       web.FS(),
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go srv.RunJanitor(ctx, 30*time.Second)

	hs := &http.Server{Addr: cfg.Addr, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()

	log.Printf("keychain 啟動於 http://%s（資料: %s）", cfg.Addr, cfg.Data)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	srv.Lock()
}
