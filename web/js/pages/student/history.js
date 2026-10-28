import { api } from "../../api.js";
import { el, pageHeader, table, badge, errorText, fmtDate, empty } from "../../utils.js";

export async function render(root) {
  const body = el("div");
  root.append(pageHeader("历史作答", "查看每次作答的成绩与 AI 报告"), body);
  try {
    const data = await api.get("/student/attempts", { page: 1, page_size: 50 });
    const items = data.items || [];
    if (!items.length) {
      body.replaceChildren(empty("还没有作答记录"));
      return;
    }
    body.replaceChildren(
      table(["试卷", "状态", "得分", "提交时间", "操作"], items, (a) =>
        el("tr", null,
          el("td", null, a.paper_title || "—"),
          el("td", null, statusBadge(a.status)),
          el("td", null, a.score != null ? `${a.score} / ${a.full_score ?? "—"}` : "—"),
          el("td", null, a.submitted_at ? fmtDate(a.submitted_at, true) : "—"),
          el("td", null,
            el("a", { class: "btn btn-sm", href: `#/student/result/${a.id}` }, "查看")
          )
        )
      )
    );
  } catch (err) {
    body.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
  }
}

function statusBadge(status) {
  const map = {
    in_progress: ["作答中", "amber"],
    submitted: ["已提交", "blue"],
    grading: ["AI 阅卷中", "amber"],
    graded: ["已阅卷", "green"],
    failed: ["失败", "red"],
  };
  const [text, tone] = map[status] || [status, ""];
  return badge(text, tone);
}
