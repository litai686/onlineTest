package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"onlineTest/internal/domain"
)

func fileDTO(f *domain.File) map[string]any {
	return map[string]any{
		"id":            f.ID,
		"original_name": f.OriginalName,
		"mime":          f.Mime,
		"size_bytes":    f.SizeBytes,
		"width":         f.Width,
		"height":        f.Height,
		"kind":          f.Kind,
		"created_at":    f.CreatedAt,
		"url":           "/api/v1/files/" + f.ID,
	}
}

func (a *App) uploadFile(w http.ResponseWriter, r *http.Request) {
	maxBytes := a.Cfg.UploadMB << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+2<<20)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, r, badRequest("上传失败："+err.Error()))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, badRequest("请选择要上传的文件"))
		return
	}
	defer file.Close()
	_ = file
	kind := strings.TrimSpace(r.FormValue("kind"))
	switch kind {
	case "", "paper_source", "question_image", "avatar":
	default:
		writeError(w, r, badRequest("无效的文件用途"))
		return
	}
	u := userFromRequest(r)
	f, err := a.Svc.SaveUpload(r.Context(), u.ID, header, kind)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, fileDTO(f))
}

func (a *App) downloadFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		writeError(w, r, badRequest("缺少文件 ID"))
		return
	}
	f, err := a.Store.GetFile(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("文件不存在"))
		return
	}
	u := userFromRequest(r)
	if u.Role == "student" {
		ok, err := a.Store.StudentCanReadFile(r.Context(), f.ID, u.ID)
		if err != nil || !ok {
			writeError(w, r, forbidden("无权访问该文件"))
			return
		}
	}
	path := a.Svc.FilePath(f)
	abs, err := filepath.Abs(path)
	if err != nil {
		writeError(w, r, err)
		return
	}
	root, _ := filepath.Abs(a.Cfg.DataDir)
	if !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		writeError(w, r, forbidden("非法文件路径"))
		return
	}
	if _, err := os.Stat(abs); err != nil {
		writeError(w, r, notFound("文件已丢失"))
		return
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", f.Mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, url.PathEscape(f.OriginalName)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, abs)
}
