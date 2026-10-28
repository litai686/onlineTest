package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"onlineTest/internal/ai"
	"onlineTest/internal/config"
	"onlineTest/internal/httpapi"
	"onlineTest/internal/security"
	"onlineTest/internal/service"
	"onlineTest/internal/store"
)

var version = "0.1.0"

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}
	setupLogger(cfg.LogLevel)
	if err := run(cfg); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func setupLogger(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}

func run(cfg config.Config) error {
	for _, dir := range []string{cfg.DataDir, filepath.Join(cfg.DataDir, "uploads"), filepath.Join(cfg.DataDir, "logs")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	secret, err := security.LoadOrCreateSecret(cfg.DataDir, cfg.MasterSecret)
	if err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "app.db"))
	if err != nil {
		return err
	}
	if err := store.Migrate(db); err != nil {
		return err
	}
	st := store.New(db)

	ctx := context.Background()
	initialPwd, generated, err := store.Seed(ctx, st, cfg.AdminPassword)
	if err != nil {
		return err
	}
	if generated {
		slog.Warn("已创建初始管理员账号，请登录后立即修改密码",
			"username", "admin", "initial_password", initialPwd)
	}

	cipher, err := security.NewCipher(secret)
	if err != nil {
		return err
	}
	jwtManager := security.NewJWTManager(secret, cfg.JWTDays)
	aiClient := ai.NewClient(st, cipher)
	svc := service.New(st, aiClient, cipher, cfg.DataDir, cfg.UploadMB<<20)
	queue := service.NewQueue(svc)
	svc.SetQueue(queue)
	queue.Start(3)
	defer queue.Stop()

	app := httpapi.New(cfg, st, jwtManager, cipher, svc, queue, version)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		WriteTimeout:      0,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", cfg.Addr, "data_dir", cfg.DataDir, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	queue.Stop()
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	return nil
}
