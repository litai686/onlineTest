import { api } from "../../api.js";
import { el, pageHeader, card, loading, toast, errorText } from "../../utils.js";

export async function render(root) {
  root.append(pageHeader("概览", "试卷、学生与 AI 状态一屏掌握"));
  const grid = el("div", { class: "grid four" });
  const recent = el("div", { class: "card" });
  root.append(grid, recent);
  grid.append(loading("加载中…"));
  try {
    const data = await api.get("/stats/overview");
    grid.replaceChildren(
      stat("学生", data.students, "人", "#/teacher/students"),
      stat("试卷", data.papers, "份", "#/teacher/papers"),
      stat("已发布", data.published, "份", "#/teacher/papers"),
      stat("今日作答", data.attempts_today, "份", "#/teacher/attempts"),
      stat("待处理任务", data.pending_jobs, "个", null),
      stat("今日 AI 调用", data.ai_calls_today, "次", "#/teacher/ai"),
      stat("今日 Token", data.ai_tokens_today, "", "#/teacher/ai")
    );
    const papers = await api.get("/papers", { page: 1, page_size: 6 });
    const paperItems = papers.items || [];
    recent.replaceChildren(
      el("div", { class: "card-head" },
        el("h2", null, "最近试卷"),
        el("a", { class: "btn btn-sm", href: "#/teacher/papers" }, "全部")
      ),
      el("div", { class: "card-body" },
        paperItems.length === 0
          ? el("p", { class: "muted" }, "还没有试卷，先去创建或上传一份吧。")
          : el("div", { class: "list" },
              ...paperItems.map((p) =>
                el("a", { class: "list-row", href: `#/teacher/papers/${p.id}` },
                  el("div", null,
                    el("strong", null, p.title),
                    el("small", { class: "muted" }, `${p.subject_name} · ${p.question_count} 题 · ${p.total_score} 分`)
                  ),
                  el("span", { class: `badge badge-${statusTone(p.status)}` }, statusText(p.status))
                )
              )
            )
      )
    );
  } catch (err) {
    grid.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
    toast(errorText(err), "error");
  }
}

function stat(label, value, unit, href) {
  const content = el("div", { class: "stat" },
    el("span", { class: "stat-label" }, label),
    el("strong", { class: "stat-value" }, Number(value || 0).toLocaleString(), unit ? el("small", null, unit) : null)
  );
  return href ? el("a", { class: "stat-card", href }, content) : el("div", { class: "stat-card" }, content);
}

function statusText(status) {
  return { draft: "草稿", ready: "待发布", published: "已发布", archived: "已归档" }[status] || status;
}
function statusTone(status) {
  return { draft: "gray", ready: "amber", published: "green", archived: "gray" }[status] || "";
}
