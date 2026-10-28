import { store } from "./store.js";
import { renderShell } from "./components/layout.js";
import { el, clear, toast, errorText } from "./utils.js";

const routes = [
  { pattern: /^\/login$/, load: () => import("./pages/login.js"), layout: null, public: true },
  { pattern: /^\/me$/, load: () => import("./pages/me.js"), layout: "self" },

  { pattern: /^\/teacher$/, load: () => import("./pages/teacher/dashboard.js"), layout: "teacher" },
  { pattern: /^\/teacher\/students$/, load: () => import("./pages/teacher/students.js"), layout: "teacher" },
  { pattern: /^\/teacher\/subjects$/, load: () => import("./pages/teacher/subjects.js"), layout: "teacher" },
  { pattern: /^\/teacher\/ai$/, load: () => import("./pages/teacher/ai.js"), layout: "teacher" },
  { pattern: /^\/teacher\/papers$/, load: () => import("./pages/teacher/papers.js"), layout: "teacher", view: "list" },
  { pattern: /^\/teacher\/papers\/new$/, load: () => import("./pages/teacher/papers.js"), layout: "teacher", view: "new" },
  { pattern: /^\/teacher\/papers\/(?<id>\d+)$/, load: () => import("./pages/teacher/papers.js"), layout: "teacher", view: "detail" },
  { pattern: /^\/teacher\/papers\/(?<id>\d+)\/edit$/, load: () => import("./pages/teacher/papers.js"), layout: "teacher", view: "edit" },
  { pattern: /^\/teacher\/attempts$/, load: () => import("./pages/teacher/attempts.js"), layout: "teacher" },

  { pattern: /^\/student$/, load: () => import("./pages/student/papers.js"), layout: "student" },
  { pattern: /^\/student\/exam\/(?<paperId>\d+)$/, load: () => import("./pages/student/exam.js"), layout: "student" },
  { pattern: /^\/student\/result\/(?<attemptId>\d+)$/, load: () => import("./pages/student/result.js"), layout: "student" },
  { pattern: /^\/student\/history$/, load: () => import("./pages/student/history.js"), layout: "student" },
];

function homeFor(role) {
  return role === "teacher" ? "/teacher" : "/student";
}

export async function router() {
  const path = (location.hash.replace(/^#/, "") || "/").split("?")[0];
  if (!store.token) {
    if (path !== "/login") {
      location.hash = "#/login";
      return;
    }
  } else if (path === "/login") {
    location.hash = `#${homeFor(store.user?.role)}`;
    return;
  }

  let matched = null;
  let params = {};
  for (const route of routes) {
    const hit = path.match(route.pattern);
    if (hit) {
      matched = route;
      params = hit.groups || {};
      break;
    }
  }

  const app = document.getElementById("app");
  if (!matched) {
    toast("页面不存在", "warn");
    location.hash = `#${homeFor(store.user?.role || "student")}`;
    return;
  }
  if (matched.layout && store.user) {
    const need = matched.layout === "self" ? store.user.role : matched.layout;
    if (store.user.role !== need) {
      location.hash = `#${homeFor(store.user.role)}`;
      return;
    }
  }

  try {
    const mod = await matched.load();
    clear(app);
    app.className = "";
    let content = app;
    if (matched.layout) {
      content = renderShell(app, matched.layout === "self" ? store.user.role : matched.layout);
    }
    const view = matched.view && mod.views ? mod.views[matched.view] : mod.render;
    if (typeof view !== "function") throw new Error("页面加载失败");
    await view(content, params);
    window.scrollTo({ top: 0 });
  } catch (err) {
    console.error(err);
    clear(app);
    app.className = "";
    app.append(
      el("div", { class: "fatal" },
        el("h2", null, "页面加载失败"),
        el("p", { class: "muted" }, errorText(err)),
        el("button", { class: "btn btn-primary", type: "button", onclick: () => location.reload() }, "重新加载")
      )
    );
  }
}
