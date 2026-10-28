package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"onlineTest/internal/filex"
	"onlineTest/internal/service"
	"onlineTest/internal/store"

	"gorm.io/gorm"
)

type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Code: 0, Message: "ok", Data: data})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	e := &AppError{Status: http.StatusInternalServerError, Code: 3001, Message: "服务器内部错误"}
	var appErr *AppError
	switch {
	case errors.As(err, &appErr):
		e = appErr
	case errors.Is(err, service.ErrNotFound), errors.Is(err, store.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		e = notFound("资源不存在")
	case errors.Is(err, service.ErrForbidden):
		e = forbidden("无权访问该资源")
	case errors.Is(err, service.ErrConflict), errors.Is(err, gorm.ErrDuplicatedKey):
		e = conflict("数据已存在或状态冲突")
	case errors.Is(err, filex.ErrTooLarge):
		e = newErr(http.StatusRequestEntityTooLarge, 1007, "文件过大")
	case errors.Is(err, filex.ErrUnsupportedType):
		e = newErr(http.StatusUnsupportedMediaType, 1007, "文件类型不支持")
	default:
		slog.Error("request failed", "path", r.URL.Path, "error", err)
	}
	body := envelope{Code: e.Code, Message: e.Message, Data: e.Data}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(body)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return badRequest("请求参数错误: " + err.Error())
	}
	return nil
}
