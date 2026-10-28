import { el, clear } from "../utils.js";
import { store } from "../store.js";

const teacherLinks = [
  { href: "#/teacher", label: "概览", icon: "◈" },
  { href: "#/teacher/papers", label: "试卷管理", icon: "▤" },
  { href: "#/teacher/attempts", label: "成绩与报告", icon: "◔" },
  { href: "#/teacher/students", label: "学生管理", icon: "☺" },
  { href: "#/teacher/subjects", label: "科目管理", icon: "▦" },
  { href: "#/teacher/ai", label: "AI 模型", icon: "✦" },
];

const studentLinks = [
  { href: "#/student", label: "试卷", icon: "▤" },
  { href: "#/student/history", label: "历史作答", icon: "◔" },
];

export function renderShell(root, role) {
  clear(root);
  const links = role === "teacher" ? teacherLinks : studentLinks;
  const current = location.hash;
  const nav = el("nav", { class: "nav" },
    ...links.map((link) => {
      const active = current === link.href || (link.href !== "#/teacher" && link.href !== "#/student" && current.startsWith(link.href));
      return el("a", { class: `nav-link ${active ? "active" : ""}`, href: link.href },
        el("span", { class: "nav-icon" }, link.icon),
        el("span", null, link.label)
      );
    })
  );

  const sidebar = el("aside", { class: "sidebar" },
    el("div", { class: "brand" },
      el("span", { class: "brand-mark" }, "题"),
      el("div", null,
        el("strong", null, "在线做题"),
        el("small", null, role === "teacher" ? "教师端" : "学生端")
      )
    ),
    nav,
    el("div", { class: "sidebar-foot" },
      el("a", { href: "#/me", class: "nav-link" }, el("span", { class: "nav-icon" }, "⚙"), el("span", null, "修改密码")),
      el("button", { class: "nav-link as-button", type: "button", onclick: logout }, el("span", { class: "nav-icon" }, "⏻"), el("span", null, "退出登录"))
    )
  );

  const user = store.user;
  const header = el("header", { class: "topbar" },
    el("button", { class: "icon-btn menu-btn", type: "button", onclick: () => sidebar.classList.toggle("open") }, "☰"),
    el("div", { class: "topbar-title" }, role === "teacher" ? "教师工作台" : "我的学习"),
    el("div", { class: "topbar-user" },
      el("span", { class: "avatar" }, (user?.display_name || user?.username || "?").slice(0, 1)),
      el("div", { class: "user-meta" },
        el("strong", null, user?.display_name || user?.username || ""),
        el("small", null, role === "teacher" ? "老师" : (user?.class_name || "学生"))
      )
    )
  );

  const content = el("main", { class: "content" });
  root.append(
    el("div", { class: "app-shell" },
      sidebar,
      el("div", { class: "main-wrap" }, header, content)
    )
  );
  return content;
}

export function logout() {
  store.logout();
  location.hash = "#/login";
}
