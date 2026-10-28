import { api } from "../../api.js";
import { el, pageHeader, badge, toast, errorText, fmtDate, empty } from "../../utils.js";

export async function render(root) {
  const body = el("div");
  root.append(pageHeader("我的试卷", "老师发布后可在线作答，交卷后由 AI 批改并生成学习建议"), body);
  try {
    const data = await api.get("/student/papers", { page: 1, page_size: 100 });
    const items = data.items || [];
    if (!items.length) {
      body.replaceChildren(empty("暂时没有已发布的试卷，等老师发布后再来看看"));
      return;
    }
    body.replaceChildren(el("div", { class: "paper-grid" },
      ...items.map((p) => paperCard(p))
    ));
  } catch (err) {
    body.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
  }
}

function paperCard(p) {
  const attempt = p.my_attempt;
  let action;
  if (!attempt) {
    action = el("a", { class: "btn btn-primary", href: `#/student/exam/${p.id}` }, "开始做题");
  } else if (attempt.status === "in_progress") {
    action = el("a", { class: "btn btn-primary", href: `#/student/exam/${p.id}` }, "继续作答");
  } else if (attempt.status === "graded") {
    action = el("a", { class: "btn", href: `#/student/result/${attempt.id}` }, "查看报告");
  } else {
    action = el("a", { class: "btn", href: `#/student/result/${attempt.id}` }, "查看进度");
  }
  return el("article", { class: "paper-card" },
    el("div", { class: "paper-card-head" },
      el("h3", null, p.title),
      statusBadge(attempt)
    ),
    el("p", { class: "muted" },
      `${p.subject_name || ""} · ${p.grade || ""}`,
      el("br"),
      `${p.question_count} 题 · ${p.total_score} 分 · ${p.duration_min ? `${p.duration_min} 分钟` : "不限时"}`
    ),
    el("div", { class: "paper-card-foot" },
      el("span", { class: "muted small" },
        attempt?.score != null ? `得分 ${attempt.score}` : attempt?.submitted_at ? `提交于 ${fmtDate(attempt.submitted_at, true)}` : `发布于 ${fmtDate(p.published_at)}`
      ),
      action
    )
  );
}

function statusBadge(attempt) {
  if (!attempt) return badge("未作答", "gray");
  const map = {
    in_progress: ["进行中", "amber"],
    submitted: ["已提交", "blue"],
    grading: ["阅卷中", "amber"],
    graded: ["已阅卷", "green"],
    failed: ["待重试", "red"],
  };
  const [text, tone] = map[attempt.status] || [attempt.status, ""];
  return badge(text, tone);
}
