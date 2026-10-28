package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"onlineTest/internal/config"
	"onlineTest/internal/domain"
	"onlineTest/internal/security"
	"onlineTest/internal/service"
	"onlineTest/internal/store"
)

type App struct {
	Cfg     config.Config
	Store   *store.Store
	JWT     *security.JWTManager
	Cipher  *security.Cipher
	Svc     *service.Service
	Queue   *service.Queue
	Version string
}

func New(cfg config.Config, st *store.Store, jwt *security.JWTManager, cipher *security.Cipher, svc *service.Service, q *service.Queue, version string) *App {
	return &App{Cfg: cfg, Store: st, JWT: jwt, Cipher: cipher, Svc: svc, Queue: q, Version: version}
}

type ctxKey string

const userKey ctxKey = "user"

func userFrom(ctx context.Context) *domain.User {
	u, _ := ctx.Value(userKey).(*domain.User)
	return u
}

func userFromRequest(r *http.Request) *domain.User { return userFrom(r.Context()) }

type AppError struct {
	Status  int
	Code    int
	Message string
	Data    any
}

func (e *AppError) Error() string { return e.Message }

func newErr(status, code int, msg string) *AppError {
	return &AppError{Status: status, Code: code, Message: msg}
}

func badRequest(msg string) *AppError { return newErr(http.StatusBadRequest, 1001, msg) }
func unauthorized(msg string) *AppError {
	return newErr(http.StatusUnauthorized, 1002, msg)
}
func forbidden(msg string) *AppError { return newErr(http.StatusForbidden, 1003, msg) }
func notFound(msg string) *AppError  { return newErr(http.StatusNotFound, 1004, msg) }
func conflict(msg string) *AppError  { return newErr(http.StatusConflict, 1005, msg) }
func invalidState(msg string) *AppError {
	return newErr(http.StatusUnprocessableEntity, 1006, msg)
}
func aiError(msg string) *AppError { return newErr(http.StatusBadGateway, 2001, msg) }

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	sqlDB, err := a.Store.DB().DB()
	if err != nil || sqlDB.Ping() != nil {
		status = "degraded"
	}
	pending, running := int64(0), int64(0)
	_ = a.Store.DB().WithContext(r.Context()).Model(&domain.Job{}).Where("status = 'pending'").Count(&pending).Error
	_ = a.Store.DB().WithContext(r.Context()).Model(&domain.Job{}).Where("status = 'running'").Count(&running).Error
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      status == "ok",
		"db":      status,
		"queue":   map[string]any{"pending": pending, "running": running},
		"version": a.Version,
	})
}

func (a *App) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version": a.Version,
		"go":      "go1.26.4",
	})
}

func pathID(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest("无效的 ID")
	}
	return id, nil
}

func intQuery(r *http.Request, key string) (int, bool) {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func pageParams(r *http.Request) (int, int) {
	page, _ := intQuery(r, "page")
	size, _ := intQuery(r, "page_size")
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func addLog(got *slog.Logger, r *http.Request, status int) {
	got.Info("http", "method", r.Method, "path", r.URL.Path, "status", status)
}
