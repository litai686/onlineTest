package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"onlineTest/internal/domain"
	"onlineTest/internal/security"
	"onlineTest/internal/store"
)

func (a *App) listStudents(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	var status *int
	if v, ok := intQuery(r, "status"); ok {
		status = &v
	}
	items, total, err := a.Store.ListStudents(r.Context(), store.StudentFilter{
		Keyword: strings.TrimSpace(r.URL.Query().Get("keyword")),
		Class:   strings.TrimSpace(r.URL.Query().Get("class_name")),
		Status:  status,
		Page:    page,
		Size:    size,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		out = append(out, userDTO(&items[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": total, "page": page, "page_size": size})
}

type createStudentRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	ClassName   string `json:"class_name"`
	Grade       string `json:"grade"`
	Phone       string `json:"phone"`
}

func (a *App) createStudent(w http.ResponseWriter, r *http.Request) {
	var req createStudentRequest
	if err := decodeJSON(w, r, &req, 64<<10); err != nil {
		writeError(w, r, err)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.Username == "" {
		writeError(w, r, badRequest("请填写用户名"))
		return
	}
	if len([]rune(req.Username)) > 64 {
		writeError(w, r, badRequest("用户名不能超过 64 个字符"))
		return
	}
	if req.DisplayName == "" || len([]rune(req.DisplayName)) > 32 {
		writeError(w, r, badRequest("姓名不能为空且不超过 32 字"))
		return
	}
	initial := ""
	pwd := req.Password
	if pwd == "" {
		var err error
		pwd, err = security.RandomString(10)
		if err != nil {
			writeError(w, r, err)
			return
		}
		initial = pwd
	} else if msg := validatePassword(pwd); msg != "" {
		writeError(w, r, badRequest(msg))
		return
	}
	hash, err := security.HashPassword(pwd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	u := domain.User{
		Username:      req.Username,
		PasswordHash:  hash,
		DisplayName:   req.DisplayName,
		Role:          "student",
		ClassName:     strings.TrimSpace(req.ClassName),
		Grade:         strings.TrimSpace(req.Grade),
		Phone:         strings.TrimSpace(req.Phone),
		Status:        1,
		MustChangePwd: initial != "",
	}
	if err := a.Store.CreateUser(r.Context(), &u); err != nil {
		writeError(w, r, err)
		return
	}
	data := map[string]any{"user": userDTO(&u)}
	if initial != "" {
		data["initial_password"] = initial
	}
	writeJSON(w, http.StatusCreated, data)
}

type updateStudentRequest struct {
	DisplayName *string `json:"display_name"`
	ClassName   *string `json:"class_name"`
	Grade       *string `json:"grade"`
	Phone       *string `json:"phone"`
	Status      *int    `json:"status"`
}

func (a *App) updateStudent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	u, err := a.Store.GetUserByID(r.Context(), id)
	if err != nil || u.Role != "student" {
		writeError(w, r, notFound("学生不存在"))
		return
	}
	var req updateStudentRequest
	if err := decodeJSON(w, r, &req, 64<<10); err != nil {
		writeError(w, r, err)
		return
	}
	fields := map[string]any{}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" {
			writeError(w, r, badRequest("姓名不能为空"))
			return
		}
		fields["display_name"] = name
	}
	if req.ClassName != nil {
		fields["class_name"] = strings.TrimSpace(*req.ClassName)
	}
	if req.Grade != nil {
		fields["grade"] = strings.TrimSpace(*req.Grade)
	}
	if req.Phone != nil {
		fields["phone"] = strings.TrimSpace(*req.Phone)
	}
	if req.Status != nil {
		if *req.Status != 0 && *req.Status != 1 {
			writeError(w, r, badRequest("状态值无效"))
			return
		}
		fields["status"] = *req.Status
		if *req.Status == 0 {
			fields["token_version"] = u.TokenVersion + 1
		}
	}
	if len(fields) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"user": userDTO(u)})
		return
	}
	if err := a.Store.UpdateUser(r.Context(), id, fields); err != nil {
		writeError(w, r, err)
		return
	}
	updated, _ := a.Store.GetUserByID(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"user": userDTO(updated)})
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

func (a *App) resetStudentPassword(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	u, err := a.Store.GetUserByID(r.Context(), id)
	if err != nil || u.Role != "student" {
		writeError(w, r, notFound("学生不存在"))
		return
	}
	var req resetPasswordRequest
	_ = decodeJSON(w, r, &req, 64<<10)
	pwd := req.NewPassword
	generated := false
	if pwd == "" {
		pwd, err = security.RandomString(10)
		if err != nil {
			writeError(w, r, err)
			return
		}
		generated = true
	} else if msg := validatePassword(pwd); msg != "" {
		writeError(w, r, badRequest(msg))
		return
	}
	hash, err := security.HashPassword(pwd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.Store.UpdateUser(r.Context(), id, map[string]any{
		"password_hash":   hash,
		"must_change_pwd": generated,
		"token_version":   u.TokenVersion + 1,
	}); err != nil {
		writeError(w, r, err)
		return
	}
	data := map[string]any{"ok": true}
	if generated {
		data["initial_password"] = pwd
	}
	writeJSON(w, http.StatusOK, data)
}

func (a *App) deleteStudent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	_, err = a.Store.GetUserByID(r.Context(), id)
	if err != nil {
		writeError(w, r, notFound("学生不存在"))
		return
	}
	var active int64
	_ = a.Store.DB().WithContext(r.Context()).Model(&domain.Attempt{}).
		Where("student_id = ? AND status IN ('in_progress','submitted','grading')", id).Count(&active).Error
	if active > 0 {
		writeError(w, r, invalidState("该学生存在进行中的答卷，请先处理后再删除"))
		return
	}
	if err := a.Store.DeleteStudent(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) listSubjects(w http.ResponseWriter, r *http.Request) {
	include := r.URL.Query().Get("include_disabled") == "1"
	items, err := a.Store.ListSubjects(r.Context(), strings.TrimSpace(r.URL.Query().Get("stage")), include)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if _, ok := intQuery(r, "status"); ok {
		filtered := items[:0]
		for _, s := range items {
			if strconv.Itoa(s.Status) == r.URL.Query().Get("status") {
				filtered = append(filtered, s)
			}
		}
		items = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type subjectRequest struct {
	Stage     string `json:"stage"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	SortOrder int    `json:"sort_order"`
	Status    *int   `json:"status"`
}

func (a *App) createSubject(w http.ResponseWriter, r *http.Request) {
	var req subjectRequest
	if err := decodeJSON(w, r, &req, 64<<10); err != nil {
		writeError(w, r, err)
		return
	}
	if req.Stage != "junior" && req.Stage != "senior" {
		writeError(w, r, badRequest("学段必须为 junior 或 senior"))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 16 {
		writeError(w, r, badRequest("科目名称不能为空且不超过 16 字"))
		return
	}
	status := 1
	if req.Status != nil {
		status = *req.Status
	}
	sub := domain.Subject{Stage: req.Stage, Name: name, Code: strings.TrimSpace(req.Code), SortOrder: req.SortOrder, Status: status}
	if err := a.Store.CreateSubject(r.Context(), &sub); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (a *App) updateSubject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	if _, err := a.Store.GetSubject(r.Context(), id); err != nil {
		writeError(w, r, notFound("科目不存在"))
		return
	}
	var req subjectRequest
	if err := decodeJSON(w, r, &req, 64<<10); err != nil {
		writeError(w, r, err)
		return
	}
	fields := map[string]any{}
	if req.Stage != "" {
		if req.Stage != "junior" && req.Stage != "senior" {
			writeError(w, r, badRequest("学段无效"))
			return
		}
		fields["stage"] = req.Stage
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		fields["name"] = name
	}
	if req.Code != "" {
		fields["code"] = strings.TrimSpace(req.Code)
	}
	if req.SortOrder != 0 {
		fields["sort_order"] = req.SortOrder
	}
	if req.Status != nil {
		fields["status"] = *req.Status
	}
	if len(fields) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if err := a.Store.UpdateSubject(r.Context(), id, fields); err != nil {
		writeError(w, r, err)
		return
	}
	sub, _ := a.Store.GetSubject(r.Context(), id)
	writeJSON(w, http.StatusOK, sub)
}

func (a *App) deleteSubject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	n, err := a.Store.CountPapersBySubject(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if n > 0 {
		writeError(w, r, invalidState("该科目下已有试卷，不能删除，可改为禁用"))
		return
	}
	if err := a.Store.DeleteSubject(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
