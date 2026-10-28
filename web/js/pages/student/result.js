import { api } from "../../api.js";
import { renderRich } from "../../components/math-render.js";
import { el, badge, toast, errorText, sleep, fmtDuration, pageHeader } from "../../utils.js";

export async function render(root, params) {
  const attemptId = params.attemptId;
  const body = el("div");
  root.append(pageHeader("成绩与报告", "AI 阅卷完成后会自动刷新"), body);
  body.append(el("p", { class: "muted" }, "加载中…"));

  for (let i = 0; i < 40; i++) {
    let data;
    try {
      data = await api.get(`/student/attempts/${attemptId}/result`);
    } catch (err) {
      body.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
      return;
    }
    renderResult(body, data);
    const status = data.attempt.status;
    if (status === "graded" || status === "failed") return;
    await sleep(3000);
  }
}

function renderResult(root, data) {
  const a = data.attempt;
  const report = data.report || {};
  const head = el("section", { class: "result-hero" },
    el("div", { class: "score-ring" },
      el("strong", null, a.score != null ? Number(a.score).toFixed(1) : "—"),
      el("small", null, ` / ${a.full_score || data.paper.total_score || "—"}`)
    ),
    el("div", { class: "result-meta" },
      el("h2", null, data.paper.title),
      el("div", { class: "badges" },
        statusBadge(a.status),
        badge(`客观题 ${a.objective_score ?? 0}`, "blue"),
        badge(`主观题 ${a.subjective_score ?? 0}`, "amber"),
        badge(`用时 ${fmtDuration(a.duration_sec)}`, "gray")
      ),
      a.grade_error ? el("p", { class: "muted small" }, a.grade_error) : null,
      a.status !== "graded" ? el("p", { class: "notice" }, "AI 正在阅卷，页面会自动更新…") : null
    )
  );

  const reportCard = el("section", { class: "card" },
    el("div", { class: "card-head" }, el("h2", null, "学习报告")),
    el("div", { class: "card-body" },
      el("p", null, report.summary || "本次作答已完成，报告生成中。"),
      (report.weak_points || []).length
        ? el("div", { class: "weak" }, ...report.weak_points.map((w) =>
            el("span", { class: "badge badge-amber" }, `${w.knowledge} 错 ${w.wrong}/${w.total}`)
          ))
        : null,
      (report.suggestions || []).length
        ? el("div", { class: "suggestions" },
            el("h4", null, "下一步建议"),
            el("ul", null, ...report.suggestions.map((s) => el("li", null, s)))
          )
        : null,
      report.ai_error ? el("p", { class: "muted small" }, `AI 报告未生成：${report.ai_error}`) : null
    )
  );

  const questionCard = el("section", { class: "card" },
    el("div", { class: "card-head" }, el("h2", null, "逐题详情")),
    el("div", { class: "card-body stack" },
      ...(data.questions || []).map((q) => questionRow(q))
    )
  );
  root.replaceChildren(head, reportCard, questionCard);
  const ring = head.querySelector(".score-ring");
  if (ring && a.full_score > 0) {
    const pct = Math.max(0, Math.min(100, ((a.score || 0) / a.full_score) * 100));
    ring.style.background = `conic-gradient(var(--primary) 0 ${pct}%, #e3e8f0 ${pct}% 100%)`;
  }
  (data.questions || []).forEach((q) => {
    const stem = root.querySelector(`[data-stem="${q.id}"]`);
    if (stem) renderRich(stem, q.stem);
    const mine = root.querySelector(`[data-mine="${q.id}"]`);
    if (mine) renderRich(mine, (q.my_answer || []).join("\n") || "（未作答）");
    const correct = root.querySelector(`[data-correct="${q.id}"]`);
    if (correct) renderRich(correct, (q.correct_answer || []).join("\n") || "—");
    const analysis = root.querySelector(`[data-analysis="${q.id}"]`);
    if (analysis) renderRich(analysis, q.analysis || "");
  });
}

function questionRow(q) {
  const graded = q.grading_mode === "ai" || q.grading_mode === "local" || q.grading_mode === "manual";
  return el("article", { class: "result-question" },
    el("div", { class: "question-head" },
      el("strong", null, `第 ${q.seq} 题`),
      badge(`${q.score ?? "—"} / ${q.max_score ?? q.score ?? "—"} 分`, q.is_correct === true ? "green" : q.is_correct === false ? "red" : "gray"),
      badge(typeText(q.type), "gray"),
      !graded ? badge("待阅", "amber") : null
    ),
    el("div", { class: "rich", dataset: { stem: q.id } }, q.stem),
    el("div", { class: "answer-block" },
      el("span", { class: "muted small" }, "我的答案"), el("div", { class: "rich", dataset: { mine: q.id } }),
      q.correct_answer ? el("span", { class: "muted small" }, "参考答案") : null,
      q.correct_answer ? el("div", { class: "rich", dataset: { correct: q.id } }) : null
    ),
    q.feedback ? el("p", { class: "feedback" }, "评语：", q.feedback) : null,
    (q.missing || []).length ? el("p", { class: "muted small" }, "遗漏：", q.missing.join("、")) : null,
    q.advice ? el("p", { class: "suggestion" }, "建议：", q.advice) : null,
    q.analysis ? el("details", { class: "analysis" }, el("summary", null, "查看解析"), el("div", { class: "rich", dataset: { analysis: q.id } })) : null
  );
}

function statusBadge(status) {
  const map = {
    in_progress: ["作答中", "amber"],
    submitted: ["已提交", "blue"],
    grading: ["AI 阅卷中", "amber"],
    graded: ["已阅卷", "green"],
    failed: ["阅卷失败", "red"],
  };
  const [text, tone] = map[status] || [status, ""];
  return badge(text, tone);
}

function typeText(t) {
  return { single: "单选", multiple: "多选", judge: "判断", fill: "填空", short: "解答" }[t] || t;
}
