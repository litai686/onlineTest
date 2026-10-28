package httpapi

import (
	"net/http"
	"strings"
	"unicode"

	"onlineTest/internal/domain"
	"onlineTest/internal/security"
)

func validatePassword(pw string) string {
	if len([]rune(pw)) < 8 || len([]rune(pw)) > 64 {
		return "密码长度需为 8-64 位"
	}
	hasLetter, hasDigit := false, false
	for _, r := range pw {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return "密码需同时包含字母和数字"
	}
	return ""
}

func userDTO(u *domain.User) map[string]any {
	return map[string]any{
		"id":              u.ID,
		"username":        u.Username,
		"display_name":    u.DisplayName,
		"role":            u.Role,
		"class_name":      u.ClassName,
		"grade":           u.Grade,
		"phone":           u.Phone,
		"status":          u.Status,
		"must_change_pwd": u.MustChangePwd,
		"last_login_at":   u.LastLoginAt,
		"created_at":      u.CreatedAt,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req, 64<<10); err != nil {
		writeError(w, r, err)
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		writeError(w, r, badRequest("请输入用户名和密码"))
		return
	}
	u, err := a.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		writeError(w, r, unauthorized("用户名或密码错误"))
		return
	}
	if u.Status != 1 {
		writeError(w, r, forbidden("账号已被禁用，请联系老师"))
		return
	}
	if !security.CheckPassword(u.PasswordHash, req.Password) {
		writeError(w, r, unauthorized("用户名或密码错误"))
		return
	}
	token, exp, err := a.JWT.Issue(u.ID, u.Role, u.DisplayName, u.TokenVersion)
	if err != nil {
		writeError(w, r, err)
		return
	}
	a.Store.TouchLogin(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": exp,
		"user":       userDTO(u),
	})
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	u := userFromRequest(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": userDTO(u)})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (a *App) changePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := decodeJSON(w, r, &req, 64<<10); err != nil {
		writeError(w, r, err)
		return
	}
	u := userFromRequest(r)
	if !security.CheckPassword(u.PasswordHash, req.OldPassword) {
		writeError(w, r, badRequest("原密码不正确"))
		return
	}
	if msg := validatePassword(req.NewPassword); msg != "" {
		writeError(w, r, badRequest(msg))
		return
	}
	if req.NewPassword == req.OldPassword {
		writeError(w, r, badRequest("新密码不能与原密码相同"))
		return
	}
	hash, err := security.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := a.Store.UpdateUser(r.Context(), u.ID, map[string]any{
		"password_hash":   hash,
		"must_change_pwd": false,
		"token_version":   u.TokenVersion + 1,
	}); err != nil {
		writeError(w, r, err)
		return
	}
	token, exp, err := a.JWT.Issue(u.ID, u.Role, u.DisplayName, u.TokenVersion+1)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires_at": exp})
}
