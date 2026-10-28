import { api } from "../../api.js";
import { renderRich } from "../../components/math-render.js";
import { createMathInput } from "../../components/math-input.js";
import { questionFigure } from "../../components/question-figure.js";
import { store } from "../../store.js";
import {
  el, clear, badge, toast, errorText, confirmDialog, sleep, debounce, fmtDuration, select,
} from "../../utils.js";

export async function render(root, params) {
  let data;
  try {
    data = await api.post(`/student/papers/${params.paperId}/attempts`);
  } catch (err) {
    root.append(el("div", { class: "error-box" }, errorText(err)),
      el("a", { class: "btn", href: "#/student" }, "返回试卷列表"));
    return;
  }
  if (data.attempt.status !== "in_progress") {
    root.append(
      el("div", { class: "notice" }, "这份试卷已经提交，不能再次作答。"),
      el("a", { class: "btn btn-primary", href: `#/student/result/${data.attempt.id}` }, "查看成绩与报告")
    );
    return;
  }

  const questions = data.questions || [];
  const answers = new Map();
  for (const a of data.answers || []) answers.set(a.question_id, [...(a.answer || [])]);
  let mode = data.attempt.mode === "practice" ? "practice" : "exam";
  const feedback = new Map();
  const dirty = new Set();
  let submitting = false;
  let savedAt = null;

  const savedLabel = el("span", { class: "muted small" }, "未修改");
  const timerLabel = el("strong", { class: "timer" }, "—");
  const submitBtn = el("button", { class: "btn btn-primary", type: "button", onclick: submit }, "交卷");
  const sheet = el("aside", { class: "answer-sheet" });
  const main = el("div", { class: "exam-main" });
  const modeSelect = select([
    { value: "exam", label: "统一交卷" },
    { value: "practice", label: "边做边看答案" },
  ], mode);
  const modeHint = el("span", { class: "muted small" },
    mode === "practice" ? "每题作答后可查看正确答案与解题思路" : "交卷后统一查看答案与解析");
  modeSelect.addEventListener("change", changeMode);

  root.append(
    el("div", { class: "exam-head" },
      el("div", null,
        el("h1", null, data.paper.title),
        el("p", { class: "muted" }, `${data.paper.duration_min ? `限时 ${data.paper.duration_min} 分钟 · ` : ""}共 ${questions.length} 题 · 满分 ${data.paper.total_score}`),
        el("div", { class: "exam-mode" },
          el("span", { class: "muted small" }, "作答模式"),
          modeSelect,
          modeHint
        )
      ),
      el("div", { class: "exam-tools" }, timerLabel, savedLabel, submitBtn)
    ),
    el("div", { class: "exam-layout" }, main, sheet)
  );

  startTimer(data.attempt.started_at, data.paper.duration_min);
  renderQuestions();
  renderSheet();

  const autoSave = debounce(flush, 1200);
  const refreshSheet = debounce(renderSheet, 200);

  function markDirty(id) {
    feedback.delete(id);
    main.querySelector(`[data-feedback="${id}"]`)?.remove();
    const checkBtn = main.querySelector(`[data-check-btn="${id}"]`);
    if (checkBtn) {
      checkBtn.disabled = false;
      checkBtn.textContent = "提交本题看答案";
    }
    dirty.add(id);
    savedLabel.textContent = "待保存…";
    savedLabel.classList.add("warn-text");
    refreshSheet();
    autoSave();
  }

  async function flush() {
    if (dirty.size === 0 || submitting) return;
    const snapshot = Array.from(dirty);
    const payload = { answers: snapshot.map((id) => ({ question_id: id, answer: answers.get(id) || [] })) };
    dirty.clear();
    try {
      const res = await api.put(`/student/attempts/${data.attempt.id}/answers`, payload);
      savedAt = res.saved_at;
      savedLabel.textContent = `已保存 ${new Date().toLocaleTimeString()}`;
      savedLabel.classList.remove("warn-text");
    } catch (err) {
      snapshot.forEach((id) => dirty.add(id));
      savedLabel.textContent = "自动保存失败，请检查网络";
      savedLabel.classList.add("warn-text");
      console.error(err);
    }
  }

  function answerBlocks(q) {
    const current = answers.get(q.id) || [];
    const update = (arr) => {
      answers.set(q.id, arr);
      markDirty(q.id);
    };
    switch (q.type) {
      case "single":
        return el("div", { class: "options-input" },
          ...q.options.map((o) =>
            el("label", { class: "option" },
              el("input", {
                type: "radio",
                name: `q-${q.id}`,
                checked: current[0] === o.key,
                onchange: () => update([o.key]),
              }),
              el("span", { class: "option-key" }, o.key),
              el("span", null, o.text)
            )
          )
        );
      case "multiple":
        return el("div", { class: "options-input" },
          ...q.options.map((o) =>
            el("label", { class: "option" },
              el("input", {
                type: "checkbox",
                checked: current.includes(o.key),
                onchange: (e) => {
                  const set = new Set(current);
                  if (e.target.checked) set.add(o.key); else set.delete(o.key);
                  update(Array.from(set).sort());
                },
              }),
              el("span", { class: "option-key" }, o.key),
              el("span", null, o.text)
            )
          )
        );
      case "judge": {
        const pick = current[0] || "";
        return el("div", { class: "options-input" },
          ...[["true", "正确"], ["false", "错误"]].map(([v, label]) =>
            el("label", { class: "option" },
              el("input", { type: "radio", name: `q-${q.id}`, checked: pick === v, onchange: () => update([v]) }),
              el("span", { class: "option-key" }, label.slice(0, 1)),
              el("span", null, label)
            )
          )
        );
      }
      case "fill": {
        const inputs = [];
        const blanks = Math.max(1, q.blanks || 1);
        for (let i = 0; i < blanks; i++) {
          const input = el("input", {
            class: "input",
            value: current[i] || "",
            placeholder: `第 ${i + 1} 空`,
            oninput: (e) => {
              const arr = [...(answers.get(q.id) || [])];
              arr[i] = e.target.value;
              answers.set(q.id, arr);
              markDirty(q.id);
            },
          });
          inputs.push(input);
        }
        return el("div", { class: "fill-inputs" }, ...inputs,
          el("button", {
            class: "btn btn-sm",
            type: "button",
            onclick: async () => {
              const latex = await promptFormula();
              if (latex) {
                const first = inputs.find((x) => !x.value) || inputs[0];
                if (first) {
                  first.value = latex;
                  first.dispatchEvent(new Event("input", { bubbles: true }));
                }
              }
            },
          }, "公式输入"));
      }
      default: {
        const editor = createMathInput({
          value: current[0] || "",
          rows: 6,
          placeholder: "解答过程，支持文字与公式混排",
          onChange: (text) => update(text ? [text] : []),
        });
        return editor.node;
      }
    }
  }

  async function promptFormula() {
    const { editFormula } = await import("../../components/math-input.js");
    return editFormula("");
  }

  function questionBlock(q, idx) {
    const card = el("article", { class: "exam-question", id: `question-${q.id}` },
      el("div", { class: "question-head" },
        el("strong", null, `第 ${q.seq} 题`),
        badge(typeText(q.type), "gray"),
        el("span", { class: "muted small" }, `${q.score} 分`)
      ),
      el("div", { class: "rich stem", dataset: { stem: q.id } }, q.stem),
      questionFigure(q.source_image),
      answerBlocks(q)
    );
    if (mode === "practice") {
      const revealed = feedback.get(q.id);
      const checkBtn = el("button", {
        class: "btn btn-sm",
        type: "button",
        dataset: { checkBtn: q.id },
        onclick: () => checkQuestion(q, checkBtn),
      }, revealed ? "重新查看答案" : "提交本题看答案");
      card.append(el("div", { class: "instant-check" }, checkBtn));
      if (revealed) card.append(feedbackPanel(q, revealed));
    }
    return card;
  }

  function renderQuestions() {
    main.replaceChildren(...questions.map((q, idx) => questionBlock(q, idx)));
    questions.forEach((q) => {
      const stem = main.querySelector(`[data-stem="${q.id}"]`);
      if (stem) renderRich(stem, q.stem);
      const analysis = main.querySelector(`[data-feedback-analysis="${q.id}"]`);
      if (analysis) renderRich(analysis, feedback.get(q.id)?.analysis || "");
    });
  }

  function answerText(q, value) {
    const list = value || [];
    if (!list.length) return "—";
    if (q.type === "judge") {
      return list.map((v) => (v === "true" ? "正确" : v === "false" ? "错误" : v)).join("、");
    }
    return list.join(q.type === "fill" ? "；" : "、");
  }

  function feedbackPanel(q, res) {
    const tone = res.is_correct === true ? "ok" : res.is_correct === false ? "bad" : "info";
    const title = res.is_correct === true ? "回答正确" : res.is_correct === false ? "回答错误" : "参考答案";
    const head = res.graded ? `${title} · 得 ${res.score} / ${res.max_score} 分` : title;
    return el("div", { class: `instant-feedback ${tone}`, dataset: { feedback: q.id } },
      el("div", { class: "instant-feedback-head" }, el("strong", null, head)),
      el("div", { class: "instant-answer" }, "正确答案：", answerText(q, res.correct_answer)),
      el("div", { class: "feedback-analysis" },
        "解题思路：",
        el("span", { dataset: { feedbackAnalysis: q.id } }, res.analysis || "暂无解析")
      )
    );
  }

  async function checkQuestion(q, btn) {
    const ans = answers.get(q.id) || [];
    if (!ans.some((s) => String(s || "").trim())) return toast("请先作答本题", "warn");
    if (btn.disabled) return;
    const original = btn.textContent;
    btn.disabled = true;
    btn.textContent = "判分中…";
    try {
      const res = await api.post(`/student/attempts/${data.attempt.id}/questions/${q.id}/check`, { answer: ans });
      feedback.set(q.id, res);
      dirty.delete(q.id);
      renderQuestions();
      toast("已给出答案与解析", "success", 3000);
    } catch (err) {
      btn.disabled = false;
      btn.textContent = original;
      toast(errorText(err), "error", 6000);
    }
  }

  async function changeMode() {
    const next = modeSelect.value;
    if (next === mode) return;
    modeSelect.disabled = true;
    try {
      await api.put(`/student/attempts/${data.attempt.id}/mode`, { mode: next });
      mode = next;
      modeHint.textContent = mode === "practice" ? "每题作答后可查看正确答案与解题思路" : "交卷后统一查看答案与解析";
      renderQuestions();
      toast(mode === "practice" ? "已切换为边做边看答案" : "已切换为统一交卷", "success", 3000);
    } catch (err) {
      modeSelect.value = mode;
      toast(errorText(err), "error", 6000);
    } finally {
      modeSelect.disabled = false;
    }
  }

  function renderSheet() {
    const answered = questions.filter((q) => {
      const ans = answers.get(q.id) || [];
      return ans.some((s) => String(s || "").trim());
    }).length;
    sheet.replaceChildren(
      el("div", { class: "sheet-head" },
        el("strong", null, "答题卡"),
        el("span", { class: "muted small" }, `${answered}/${questions.length}`)
      ),
      el("div", { class: "sheet-grid" },
        ...questions.map((q) => {
          const ans = answers.get(q.id) || [];
          const done = ans.some((s) => String(s || "").trim());
          return el("button", {
            class: `sheet-cell ${done ? "done" : ""}`,
            type: "button",
            onclick: () => document.getElementById(`question-${q.id}`)?.scrollIntoView({ behavior: "smooth", block: "start" }),
          }, String(q.seq));
        })
      ),
      el("p", { class: "muted small" }, "点击题号快速定位；离开页面前会自动保存。"),
      submitBtn.cloneNode(true)
    );
    const cloneSubmit = sheet.querySelector("button.btn-primary");
    if (cloneSubmit) cloneSubmit.addEventListener("click", submit);
  }

  function startTimer(startedAt, minutes) {
    if (!minutes) {
      timerLabel.textContent = "不限时";
      return;
    }
    const end = new Date(startedAt).getTime() + minutes * 60000;
    const tick = () => {
      const remain = Math.max(0, Math.floor((end - Date.now()) / 1000));
      timerLabel.textContent = remain > 0 ? `剩余 ${fmtDuration(remain)}` : "已超时";
      timerLabel.classList.toggle("danger-text", remain <= 0);
    };
    tick();
    setInterval(tick, 1000);
  }

  async function submit() {
    if (submitting) return;
    if (!(await confirmDialog("确定交卷？交卷后无法修改答案。", { confirmText: "交卷" }))) return;
    submitting = true;
    submitBtn.disabled = true;
    autoSave.cancel();
    await flush();
    try {
      const res = await api.post(`/student/attempts/${data.attempt.id}/submit`, { confirm: true });
      if (res.status === "graded") {
        location.hash = `#/student/result/${data.attempt.id}`;
        return;
      }
      const overlay = el("div", { class: "grading-overlay" },
        el("div", { class: "grading-card" },
          el("div", { class: "spinner" }),
          el("h3", null, "AI 阅卷中…"),
          el("p", { class: "muted" }, "客观题已判分，解答题正在分析，请稍候"),
          el("p", { class: "muted small", id: "grade-progress" }, "正在排队")
        )
      );
      root.append(overlay);
      if (res.job_id) {
        for (let i = 0; i < 200; i++) {
          const job = await api.get(`/jobs/${res.job_id}`);
          const p = overlay.querySelector("#grade-progress");
          if (p) p.textContent = `${job.message || "阅卷中"}（${job.progress || 0}%）`;
          if (["succeeded", "failed", "canceled"].includes(job.status)) break;
          await sleep(2000);
        }
      } else {
        await sleep(1500);
      }
      location.hash = `#/student/result/${data.attempt.id}`;
    } catch (err) {
      toast(errorText(err), "error", 6000);
      submitting = false;
      submitBtn.disabled = false;
    }
  }

  window.addEventListener("pagehide", () => {
    if (!dirty.size) return;
    const payload = JSON.stringify({ answers: Array.from(dirty).map((id) => ({ question_id: id, answer: answers.get(id) || [] })) });
    fetch(`/api/v1/student/attempts/${data.attempt.id}/answers`, {
      method: "PUT",
      keepalive: true,
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${store.token}` },
      body: payload,
    }).catch(() => {});
  }, { once: true });
}

function typeText(t) {
  return { single: "单选", multiple: "多选", judge: "判断", fill: "填空", short: "解答" }[t] || t;
}
