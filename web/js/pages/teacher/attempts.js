import { api } from "../../api.js";
import { renderRich } from "../../components/math-render.js";
import { questionFigure } from "../../components/question-figure.js";
import { el, pageHeader, table, badge, modal, field, select, toast, errorText, fmtDate } from "../../utils.js";

export async function render(root) {
  const papers = await api.get("/papers", { page: 1, page_size: 100 });
  const paperItems = papers.items || [];
  const state = { page: 1, paper_id: "", status: "" };
  const paperSelect = select(
    [{ value: "", label: "全部试卷" }, ...paperItems.map((p) => ({ value: String(p.id), label: p.title }))],
    ""
  );
  const status = select([
    { value: "", label: "全部状态" },
    { value: "in_progress", label: "进行中" },
    { value: "grading", label: "阅卷中" },
    { value: "graded", label: "已阅卷" },
  ], "");
  const filters = el("form", { class: "filters" },
    paperSelect, status,
    el("button", { class: "btn btn-primary", type: "submit" }, "筛选")
  );
  const summary = el("div", { class: "grid four" });
  const body = el("div");
  root.append(pageHeader("成绩与报告", "查看学生作答、AI 评语与整卷建议；人工复核可改分"), filters, summary, body);

  filters.addEventListener("submit", (e) => {
    e.preventDefault();
    state.paper_id = paperSelect.value;
    state.status = status.value;
    state.page = 1;
    load();
  });

  async function load() {
    body.replaceChildren(el("p", { class: "muted" }, "加载中…"));
    try {
      const data = await api.get("/teacher/attempts", {
        page: state.page, page_size: 20, paper_id: state.paper_id, status: state.status,
      });
      if (data.summary) {
        summary.replaceChildren(
          stat("提交", data.summary.submitted),
          stat("已阅", data.summary.graded),
          stat("平均分", Number(data.summary.avg_score || 0).toFixed(1))
        );
      } else {
        summary.replaceChildren();
      }
      body.replaceChildren(
        table(["学生", "试卷", "状态", "得分", "提交时间", "操作"], data.items, (a) =>
          el("tr", null,
            el("td", null, `${a.student_name || "—"}`, a.class_name ? el("small", { class: "muted" }, ` · ${a.class_name}`) : null),
            el("td", null, a.paper_title || "—"),
            el("td", null, statusBadge(a.status)),
            el("td", null, `${a.score ?? "—"} / ${a.full_score ?? "—"}`),
            el("td", null, a.submitted_at ? fmtDate(a.submitted_at, true) : "—"),
            el("td", null,
              el("button", { class: "btn btn-sm", type: "button", onclick: () => openReport(a.id) }, "查看报告"),
              el("button", { class: "btn btn-sm", type: "button", onclick: () => regrade(a.id) }, "重新阅卷")
            )
          )
        ),
        pager(data.total, state.page, (p) => { state.page = p; load(); })
      );
    } catch (err) {
      body.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
    }
  }

  async function openReport(id) {
    const container = el("div", { class: "stack" }, el("p", { class: "muted" }, "加载中…"));
    const handle = modal({ title: "作答详情", width: 900, content: container, actions: [{ text: "关闭" }] });
    try {
      const data = await api.get(`/teacher/attempts/${id}/result`);
      const attempt = data.attempt;
      const report = data.report || {};
      const head = el("div", { class: "result-head" },
        el("div", { class: "score-ring" },
          el("strong", null, attempt.score ?? "—"),
          el("small", null, ` / ${attempt.full_score ?? "—"}`)
        ),
        el("div", null,
          el("h3", null, data.paper?.title || ""),
          el("p", { class: "muted" },
            `${data.student?.display_name || ""} · ${statusText(attempt.status)} · 用时 ${Math.round((attempt.duration_sec || 0) / 60)} 分钟`
          ),
          el("p", { class: "muted small" }, attempt.grade_error || "")
        )
      );
      const pct = attempt.full_score > 0 ? Math.max(0, Math.min(100, ((attempt.score || 0) / attempt.full_score) * 100)) : 0;
      head.querySelector(".score-ring").style.background = `conic-gradient(var(--primary) 0 ${pct}%, #e3e8f0 ${pct}% 100%)`;
      const reportBox = el("div", { class: "report" },
        el("h4", null, "学习报告"),
        el("p", null, report.summary || "暂无总结"),
        (report.weak_points || []).length
          ? el("div", { class: "weak" }, ...report.weak_points.map((w) =>
              el("span", { class: "badge badge-amber" }, `${w.knowledge} 错 ${w.wrong}/${w.total}`)
            ))
          : null,
        (report.suggestions || []).length
          ? el("ul", null, ...report.suggestions.map((s) => el("li", null, s)))
          : null
      );
      const questions = el("div", { class: "stack" }, ...(data.questions || []).map((q) => resultQuestion(q, data, handle)));
      container.replaceChildren(head, reportBox, el("h4", null, "逐题详情"), questions);
      (data.questions || []).forEach((q) => {
        const stem = container.querySelector(`[data-q-stem="${q.id}"]`);
        if (stem) renderRich(stem, q.stem);
        const mine = container.querySelector(`[data-q-mine="${q.id}"]`);
        if (mine) renderRich(mine, (q.my_answer || []).join("\n") || "（未作答）");
        const correct = container.querySelector(`[data-q-correct="${q.id}"]`);
        if (correct) renderRich(correct, (q.correct_answer || []).join("\n") || "—");
      });
    } catch (err) {
      container.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
    }
  }

  function resultQuestion(q, data, handle) {
    const manual = data.manual_review?.find((m) => m.question_id === q.id);
    const scoreInput = manual ? el("input", { class: "input", type: "number", step: "0.5", min: "0", max: q.max_score, value: q.score ?? 0 }) : null;
    const feedback = manual ? el("input", { class: "input", value: q.feedback || "" }) : null;
    return el("div", { class: "result-question" },
      el("div", { class: "question-head" },
        el("strong", null, `第 ${q.seq} 题`),
        badge(`${q.score ?? "—"} / ${q.max_score ?? "—"} 分`, q.is_correct ? "green" : "amber"),
        badge(q.grading_mode === "ai" ? "AI 阅卷" : q.grading_mode === "manual" ? "人工" : "自动", "gray")
      ),
      el("div", { class: "rich", dataset: { qStem: q.id } }, q.stem),
      questionFigure(q.source_image),
      el("div", { class: "answer-block" },
        el("span", { class: "muted small" }, "学生答案"), el("div", { class: "rich", dataset: { qMine: q.id } }),
        el("span", { class: "muted small" }, "参考答案"), el("div", { class: "rich", dataset: { qCorrect: q.id } })
      ),
      q.feedback ? el("p", { class: "feedback" }, "评语：", q.feedback) : null,
      (q.missing || []).length ? el("p", { class: "muted small" }, "遗漏：", q.missing.join("、")) : null,
      q.advice ? el("p", { class: "muted small" }, "建议：", q.advice) : null,
      manual
        ? el("div", { class: "manual-score" },
            field(`人工改分（满分 ${q.max_score}）`, scoreInput),
            field("评语", feedback),
            el("button", {
              class: "btn btn-sm btn-primary",
              type: "button",
              onclick: async () => {
                try {
                  await api.put(`/teacher/attempts/${data.attempt.id}/answers/${q.id}`, {
                    score: Number(scoreInput.value),
                    feedback: feedback.value,
                  });
                  toast("已保存并重新汇总", "success");
                  handle.close();
                  load();
                } catch (err) {
                  toast(errorText(err), "error");
                }
              },
            }, "保存评分")
          )
        : null
    );
  }

  async function regrade(id) {
    try {
      await api.post(`/teacher/attempts/${id}/regrade`);
      toast("已提交重新阅卷", "success");
      load();
    } catch (err) {
      toast(errorText(err), "error");
    }
  }

  load();
}

function statusBadge(status) {
  const map = {
    in_progress: ["进行中", "gray"],
    submitted: ["已提交", "blue"],
    grading: ["阅卷中", "amber"],
    graded: ["已阅卷", "green"],
    failed: ["失败", "red"],
  };
  const [text, tone] = map[status] || [status, ""];
  return badge(text, tone);
}
function statusText(s) {
  return { in_progress: "进行中", submitted: "已提交", grading: "阅卷中", graded: "已阅卷", failed: "失败" }[s] || s;
}

function stat(label, value) {
  return el("div", { class: "stat-card" },
    el("div", { class: "stat" },
      el("span", { class: "stat-label" }, label),
      el("strong", { class: "stat-value" }, value)
    )
  );
}

function pager(total, page, onChange, size = 20) {
  const pages = Math.max(1, Math.ceil(total / size));
  if (pages <= 1) return el("div");
  return el("div", { class: "pagination" },
    el("button", { class: "btn btn-sm", type: "button", disabled: page <= 1, onclick: () => onChange(page - 1) }, "上一页"),
    el("span", { class: "muted" }, `${page} / ${pages}`),
    el("button", { class: "btn btn-sm", type: "button", disabled: page >= pages, onclick: () => onChange(page + 1) }, "下一页")
  );
}
