import { api } from "../api.js";
import { store } from "../store.js";
import { el, clear, field, toast, errorText, pageHeader } from "../utils.js";

export async function render(root) {
  const current = el("input", { class: "input", type: "password", autocomplete: "current-password", placeholder: "当前密码" });
  const next = el("input", { class: "input", type: "password", autocomplete: "new-password", placeholder: "新密码（8-64 位，含字母和数字）" });
  const repeat = el("input", { class: "input", type: "password", autocomplete: "new-password", placeholder: "再次输入新密码" });
  const submit = el("button", { class: "btn btn-primary", type: "submit" }, "保存新密码");
  const form = el("form", { class: "stack" },
    field("当前密码", current),
    field("新密码", next),
    field("确认新密码", repeat),
    submit
  );
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (next.value !== repeat.value) {
      toast("两次输入的新密码不一致", "warn");
      return;
    }
    submit.disabled = true;
    try {
      const data = await api.post("/auth/change-password", {
        old_password: current.value,
        new_password: next.value,
      });
      store.setAuth(data.token, store.user);
      toast("密码已更新，其他设备需重新登录", "success");
      form.reset();
    } catch (err) {
      toast(errorText(err), "error");
    } finally {
      submit.disabled = false;
    }
  });

  const user = store.user;
  root.append(
    pageHeader("账号设置"),
    el("div", { class: "grid two" },
      el("section", { class: "card" },
        el("div", { class: "card-head" }, el("h2", null, "个人信息")),
        el("div", { class: "card-body" },
          el("dl", { class: "meta-list" },
            el("dt", null, "用户名"), el("dd", null, user?.username || ""),
            el("dt", null, "姓名"), el("dd", null, user?.display_name || ""),
            el("dt", null, "角色"), el("dd", null, user?.role === "teacher" ? "老师" : "学生"),
            el("dt", null, "班级"), el("dd", null, user?.class_name || "—"),
            el("dt", null, "年级"), el("dd", null, user?.grade || "—")
          )
        )
      ),
      el("section", { class: "card" },
        el("div", { class: "card-head" }, el("h2", null, "修改密码")),
        el("div", { class: "card-body" }, form)
      )
    )
  );
}
