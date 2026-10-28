package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

const reqIDKey ctxKey = "request_id"

func requestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := requestID()
		ctx := context.WithValue(r.Context(), reqIDKey, rid)
		r = r.WithContext(ctx)
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "error", rec, "path", r.URL.Path, "request_id", rid, "stack", string(debug.Stack()))
				writeError(sw, r, fmt.Errorf("panic: %v", rec))
			}
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			slog.Info("http",
				"request_id", rid,
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()
		next.ServeHTTP(sw, r.WithContext(ctx))
	})
}

func (a *App) authed(next http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, r, unauthorized("请先登录"))
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := a.JWT.Parse(token)
		if err != nil {
			writeError(w, r, unauthorized("登录已过期，请重新登录"))
			return
		}
		id, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			writeError(w, r, unauthorized("登录凭证无效"))
			return
		}
		u, err := a.Store.GetUserByID(r.Context(), id)
		if err != nil || u.Status != 1 {
			writeError(w, r, unauthorized("账号不存在或已被禁用"))
			return
		}
		if u.TokenVersion != claims.Ver {
			writeError(w, r, unauthorized("登录状态已失效，请重新登录"))
			return
		}
		if len(roles) > 0 {
			ok := false
			for _, role := range roles {
				if u.Role == role {
					ok = true
					break
				}
			}
			if !ok {
				writeError(w, r, forbidden("无权限执行该操作"))
				return
			}
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next(w, r.WithContext(ctx))
	}
}

func (a *App) teacher(next http.HandlerFunc) http.HandlerFunc { return a.authed(next, "teacher") }
func (a *App) student(next http.HandlerFunc) http.HandlerFunc { return a.authed(next, "student") }
