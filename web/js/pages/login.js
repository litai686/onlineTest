import { api } from "../api.js";
import { store } from "../store.js";
import { el, field, toast, errorText } from "../utils.js";

export async function render(root) {
  root.className = "login-page";
  const username = el("input", { class: "input", autocomplete: "username", placeholder: "用户名" });
  const password = el("input", { class: "input", type: "password", autocomplete: "current-password", placeholder: "密码" });
  const submit = el("button", { class: "btn btn-primary btn-block", type: "submit" }, "登录");
  const error = el("div", { class: "form-error hidden" });
  const form = el("form", { class: "login-card" },
    el("div", { class: "login-brand" },
      el("span", { class: "brand-mark big" }, "题"),
      el("h1", null, "在线做题系统"),
      el("p", { class: "muted" }, "AI 识别试卷 · 自动阅卷 · 学情建议")
    ),
    field("用户名", username),
    field("密码", password),
    error,
    submit,
    el("p", { class: "login-tip muted" }, "学生账号由老师创建，无需注册")
  );
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    error.classList.add("hidden");
    if (!username.value.trim() || !password.value) {
      error.textContent = "请输入用户名和密码";
      error.classList.remove("hidden");
      return;
    }
    submit.disabled = true;
    submit.textContent = "登录中…";
    try {
      const data = await api.post("/auth/login", {
        username: username.value.trim(),
        password: password.value,
      });
      store.setAuth(data.token, data.user);
      toast(`欢迎回来，${data.user.display_name || data.user.username}`, "success");
      location.hash = data.user.role === "teacher" ? "#/teacher" : "#/student";
    } catch (err) {
      error.textContent = errorText(err);
      error.classList.remove("hidden");
    } finally {
      submit.disabled = false;
      submit.textContent = "登录";
    }
  });
  root.append(
    el("div", { class: "login-bg" },
      el("div", { class: "login-orbs" }),
      form
    )
  );
}
