import { api } from "../../api.js";
import { el, pageHeader, table, badge, modal, field, select, toast, errorText, confirmDialog, empty } from "../../utils.js";

export async function render(root) {
  const state = { page: 1, keyword: "", class: "", status: "" };
  const body = el("div");
  const keyword = el("input", { class: "input", placeholder: "搜索用户名 / 姓名" });
  const status = select([{ value: "", label: "全部状态" }, { value: "1", label: "启用" }, { value: "0", label: "禁用" }], "");
  const search = el("form", { class: "filters" },
    keyword, status,
    el("button", { class: "btn btn-primary", type: "submit" }, "搜索"),
    el("button", { class: "btn", type: "button", onclick: () => openForm() }, "新增学生")
  );
  search.addEventListener("submit", (e) => {
    e.preventDefault();
    state.keyword = keyword.value.trim();
    state.status = status.value;
    state.page = 1;
    load();
  });
  root.append(pageHeader("学生管理", "学生账号由老师创建，无需注册"), search, body);

  async function load() {
    body.replaceChildren(el("p", { class: "muted" }, "加载中…"));
    try {
      const data = await api.get("/students", {
        page: state.page,
        page_size: 20,
        keyword: state.keyword,
        status: state.status,
      });
      const items = data.items || [];
      if (items.length === 0) {
        body.replaceChildren(empty("暂无学生，点击右上角新增"));
        return;
      }
      body.replaceChildren(
        table(["用户名", "姓名", "班级", "状态", "最后登录", "操作"], items, (u) =>
          el("tr", null,
            el("td", null, el("code", null, u.username)),
            el("td", null, u.display_name),
            el("td", null, u.class_name || "—"),
            el("td", null, badge(u.status === 1 ? "启用" : "禁用", u.status === 1 ? "green" : "gray")),
            el("td", null, u.last_login_at ? new Date(u.last_login_at).toLocaleString() : "—"),
            el("td", { class: "actions" },
              el("button", { class: "btn btn-sm", type: "button", onclick: () => openForm(u) }, "编辑"),
              el("button", { class: "btn btn-sm", type: "button", onclick: () => resetPassword(u) }, "重置密码"),
              el("button", { class: "btn btn-sm btn-danger", type: "button", onclick: () => remove(u) }, "删除")
            )
          )
        ),
        pagination(data.total, state.page, 20, (p) => { state.page = p; load(); })
      );
    } catch (err) {
      body.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
    }
  }

  function openForm(user = null) {
    const username = el("input", { class: "input", value: user?.username || "", disabled: Boolean(user) });
    const name = el("input", { class: "input", value: user?.display_name || "" });
    const className = el("input", { class: "input", value: user?.class_name || "" });
    const grade = el("input", { class: "input", value: user?.grade || "" });
    const password = el("input", { class: "input", type: "text", placeholder: "留空则自动生成" });
    const enabled = select([{ value: "1", label: "启用" }, { value: "0", label: "禁用" }], String(user?.status ?? 1));
    const content = el("div", { class: "stack" },
      field("用户名", username, user ? "用户名不可修改" : "登录账号，支持中文、字母、数字和符号"),
      field("姓名", name),
      field("班级", className),
      field("年级", grade, "如：初二、高一"),
      user ? null : field("初始密码", password, "留空则自动生成并显示一次"),
      user ? field("状态", enabled) : null
    );
    modal({
      title: user ? "编辑学生" : "新增学生",
      content,
      actions: [
        { text: "取消" },
        {
          text: user ? "保存" : "创建",
          primary: true,
          onClick: async (close) => {
            try {
              if (!user) {
                if (!username.value.trim()) {
                  toast("请填写用户名", "warn");
                  username.focus();
                  return false;
                }
                if (!name.value.trim()) {
                  toast("请填写学生姓名", "warn");
                  name.focus();
                  return false;
                }
                if (password.value && password.value.length < 8) {
                  toast("初始密码至少 8 位，建议包含字母和数字", "warn");
                  password.focus();
                  return false;
                }
              }
              if (user) {
                await api.put(`/students/${user.id}`, {
                  display_name: name.value.trim(),
                  class_name: className.value.trim(),
                  grade: grade.value.trim(),
                  status: Number(enabled.value),
                });
                toast("已保存", "success");
              } else {
                const data = await api.post("/students", {
                  username: username.value.trim(),
                  display_name: name.value.trim(),
                  class_name: className.value.trim(),
                  grade: grade.value.trim(),
                  password: password.value,
                });
                const pwd = data.initial_password;
                toast(pwd ? `创建成功，初始密码：${pwd}` : "创建成功", "success", 8000);
              }
              close();
              load();
            } catch (err) {
              toast(errorText(err), "error");
              return false;
            }
          },
        },
      ],
    });
  }

  async function resetPassword(user) {
    const input = el("input", { class: "input", placeholder: "留空则自动生成" });
    modal({
      title: `重置 ${user.display_name} 的密码`,
      content: el("div", { class: "stack" }, field("新密码", input, "重置后该学生需要用新密码重新登录")),
      actions: [
        { text: "取消" },
        {
          text: "重置",
          primary: true,
          onClick: async (close) => {
            try {
              const data = await api.post(`/students/${user.id}/reset-password`, { new_password: input.value });
              toast(data.initial_password ? `新密码：${data.initial_password}` : "密码已重置", "success", 8000);
              close();
            } catch (err) {
              toast(errorText(err), "error");
              return false;
            }
          },
        },
      ],
    });
  }

  async function remove(user) {
    if (!(await confirmDialog(`确定删除学生「${user.display_name}」？删除后无法登录，历史作答保留。`, { danger: true, confirmText: "删除" }))) return;
    try {
      await api.del(`/students/${user.id}`);
      toast("已删除", "success");
      load();
    } catch (err) {
      toast(errorText(err), "error");
    }
  }

  load();
}

function pagination(total, page, size, onChange) {
  const pages = Math.max(1, Math.ceil(total / size));
  if (pages <= 1) return el("div");
  return el("div", { class: "pagination" },
    el("button", { class: "btn btn-sm", disabled: page <= 1, type: "button", onclick: () => onChange(page - 1) }, "上一页"),
    el("span", { class: "muted" }, `${page} / ${pages}，共 ${total} 条`),
    el("button", { class: "btn btn-sm", disabled: page >= pages, type: "button", onclick: () => onChange(page + 1) }, "下一页")
  );
}
