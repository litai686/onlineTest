import { api } from "../../api.js";
import { renderRich, richInline } from "../../components/math-render.js";
import { createMathInput } from "../../components/math-input.js";
import { createUploader } from "../../components/uploader.js";
import { questionFigure } from "../../components/question-figure.js";
import {
  el, clear, pageHeader, table, badge, modal, field, select, toast, errorText,
  confirmDialog, empty, sleep, fmtDate,
} from "../../utils.js";

let subjectsCache = [];

async function loadSubjects() {
  if (subjectsCache.length) return subjectsCache;
  const data = await api.get("/subjects", { include_disabled: "1" });
  subjectsCache = data.items;
  return subjectsCache;
}

function subjectOptions(includeAll = false) {
  const opts = subjectsCache.map((s) => ({
    value: String(s.id),
    label: `${s.stage === "junior" ? "初中" : "高中"} · ${s.name}${s.status === 1 ? "" : "（禁用）"}`,
  }));
  return includeAll ? [{ value: "", label: "全部科目" }, ...opts] : opts;
}

function statusText(status) {
  return { draft: "草稿", ready: "待发布", published: "已发布", archived: "已归档" }[status] || status;
}
function statusTone(status) {
  return { draft: "gray", ready: "amber", published: "green", archived: "gray" }[status] || "";
}

export const views = { list, new: newPaper, detail, edit };

async function list(root, params = {}) {
  await loadSubjects();
  const state = { page: 1, keyword: "", status: "", subject_id: "" };
  const keyword = el("input", { class: "input", placeholder: "搜索试卷标题" });
  const status = select([
    { value: "", label: "全部状态" },
    { value: "draft", label: "草稿" },
    { value: "ready", label: "待发布" },
    { value: "published", label: "已发布" },
    { value: "archived", label: "已归档" },
  ], "");
  const subject = select(subjectOptions(true), "");
  const search = el("form", { class: "filters" },
    keyword, subject, status,
    el("button", { class: "btn btn-primary", type: "submit" }, "筛选"),
    el("a", { class: "btn", href: "#/teacher/papers/new" }, "新建 / 上传试卷")
  );
  search.addEventListener("submit", (e) => {
    e.preventDefault();
    state.keyword = keyword.value.trim();
    state.status = status.value;
    state.subject_id = subject.value;
    state.page = 1;
    load();
  });
  const body = el("div");
  root.append(pageHeader("试卷管理", "上传图片或 PDF，由 AI 识别成结构化题目后校对发布"), search, body);

  async function load() {
    body.replaceChildren(el("p", { class: "muted" }, "加载中…"));
    try {
      const data = await api.get("/papers", {
        page: state.page, page_size: 20,
        keyword: state.keyword, status: state.status, subject_id: state.subject_id,
      });
      const items = data.items || [];
      if (!items.length) {
        body.replaceChildren(empty("暂无试卷，点击「新建 / 上传试卷」开始"));
        return;
      }
      body.replaceChildren(
        table(["试卷", "科目", "题数/总分", "状态", "解析", "作答", "创建时间", "操作"], items, (p) =>
          el("tr", null,
            el("td", null, el("a", { class: "link", href: `#/teacher/papers/${p.id}` }, p.title)),
            el("td", null, p.subject_name || "—"),
            el("td", null, `${p.question_count} 题 / ${p.total_score} 分`),
            el("td", null, badge(statusText(p.status), statusTone(p.status))),
            el("td", null, badge(parseText(p.parse_status), parseTone(p.parse_status))),
            el("td", null, `${p.attempt_count}（${p.graded_count} 已阅）`),
            el("td", null, fmtDate(p.created_at)),
            el("td", { class: "actions" },
              el("a", { class: "btn btn-sm", href: `#/teacher/papers/${p.id}` }, "查看"),
              el("a", { class: "btn btn-sm", href: `#/teacher/papers/${p.id}/edit` }, "编辑")
            )
          )
        ),
        pager(data.total, state.page, (p) => { state.page = p; load(); })
      );
    } catch (err) {
      body.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
    }
  }
  load();
}

function parseText(s) {
  return { none: "未解析", queued: "排队中", running: "解析中", succeeded: "成功", failed: "失败" }[s] || s;
}
function parseTone(s) {
  return { succeeded: "green", failed: "red", running: "blue", queued: "amber" }[s] || "gray";
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

async function newPaper(root) {
  await loadSubjects();
  const title = el("input", { class: "input", placeholder: "如：初二数学期中试卷", value: "" });
  const subject = select(subjectOptions(false), subjectsCache[0]?.id || "");
  const grade = el("input", { class: "input", placeholder: "如：初二 / 高一" });
  const duration = el("input", { class: "input", type: "number", value: 120, min: "0" });
  const aiGrading = el("input", { type: "checkbox", checked: true });
  const showAnswer = el("input", { type: "checkbox", checked: true });
  const uploader = createUploader({ hint: "点击选择，或截图后直接 Ctrl+V 粘贴到此处" });
  const hint = el("textarea", { class: "input", rows: 3, placeholder: "可选：告诉 AI 例如「共 23 题，含 6 道解答题」" });
  const progress = el("div", { class: "parse-progress hidden" },
    el("div", { class: "progress" }, el("div", { class: "progress-bar", style: { width: "0%" } })),
    el("p", { class: "muted" }, "")
  );
  const progressBar = progress.querySelector(".progress-bar");
  const progressText = progress.querySelector("p");
  let busy = false;

  const form = el("section", { class: "card" },
    el("div", { class: "card-head" }, el("h2", null, "基本信息")),
    el("div", { class: "card-body stack" },
      el("div", { class: "grid two" }, field("试卷标题", title), field("科目", subject)),
      el("div", { class: "grid three" },
        field("年级", grade),
        field("限时（分钟，0 表示不限）", duration),
        el("div", { class: "checks stack-tight" },
          el("label", { class: "check" }, aiGrading, "主观题 AI 阅卷"),
          el("label", { class: "check" }, showAnswer, "交卷后展示答案")
        )
      )
    )
  );

  const aiBtn = el("button", { class: "btn btn-primary", type: "button", onclick: () => submit(true) }, "创建并 AI 识别");
  const manualBtn = el("button", { class: "btn", type: "button", onclick: () => submit(false) }, "仅创建空白试卷");

  const upload = el("section", { class: "card" },
    el("div", { class: "card-head" }, el("h2", null, "上传试卷")),
    el("div", { class: "card-body stack" },
      field("试卷文件", uploader.node, "可一次选择多张图片或多个 PDF；创建后可在详情页继续增量上传"),
      field("识别提示", hint),
      progress,
      el("div", { class: "actions" }, aiBtn, manualBtn)
    )
  );

  root.append(
    pageHeader("新建试卷", "可直接上传试卷让 AI 识别，也可以先创建空白试卷"),
    form, upload
  );

  async function submit(withAI) {
    if (busy) return;
    const name = title.value.trim();
    if (!name) return toast("请填写试卷标题", "warn");
    if (!subject.value) return toast("请选择科目", "warn");
    if (withAI && uploader.getFiles().length === 0) return toast("请选择或粘贴试卷文件", "warn");

    const activeBtn = withAI ? aiBtn : manualBtn;
    const originalText = activeBtn.textContent;
    let createdPaper = null;
    let navigating = false;
    busy = true;
    aiBtn.disabled = true;
    manualBtn.disabled = true;
    try {
      createdPaper = await api.post("/papers", {
        title: name,
        subject_id: Number(subject.value),
        grade: grade.value.trim(),
        duration_min: Number(duration.value) || 0,
        ai_grading: aiGrading.checked,
        show_answer: showAnswer.checked,
      });
      if (!withAI) {
        navigating = true;
        toast("已创建，请上传题目截图", "success");
        location.hash = `#/teacher/papers/${createdPaper.id}/edit`;
        return;
      }

      const picked = uploader.getFiles();
      const totalMB = (picked.reduce((sum, f) => sum + f.size, 0) / 1024 / 1024).toFixed(1);
      activeBtn.textContent = "正在上传…";
      progress.classList.remove("hidden");
      progressBar.style.width = "8%";
      progressText.textContent = `正在上传 ${picked.length} 个文件（${totalMB} MB）…`;

      const formData = new FormData();
      for (const f of picked) formData.append("files", f);
      formData.append("hint", hint.value.trim());
      formData.append("mode", "replace");
      const res = await api.form(`/papers/${createdPaper.id}/parse`, "POST", formData);

      activeBtn.textContent = "AI 正在识别…";
      progressText.textContent = "文件已上传，AI 正在识别题目…";
      progressBar.style.width = "10%";
      const job = await pollJob(res.job_id, (j) => {
        const pct = Math.max(10, j.progress || 0);
        progressBar.style.width = `${pct}%`;
        progressText.textContent = `${j.message || "AI 识别中"}（${pct}%）`;
        activeBtn.textContent = jobStageText(j);
      });
      if (job.status === "succeeded") {
        navigating = true;
        activeBtn.textContent = "识别完成";
        toast(job.message || "识别完成，请校对题目", "success", 6000);
        setTimeout(() => { location.hash = `#/teacher/papers/${createdPaper.id}/edit`; }, 1000);
      } else {
        navigating = true;
        activeBtn.textContent = "识别失败";
        toast(job.last_error || "识别失败", "error", 8000);
        setTimeout(() => { location.hash = `#/teacher/papers/${createdPaper.id}`; }, 1500);
      }
    } catch (err) {
      toast(errorText(err), "error", 8000);
      if (createdPaper) {
        navigating = true;
        progress.classList.remove("hidden");
        progressText.textContent = `上传或识别失败：${errorText(err)}`;
        setTimeout(() => { location.hash = `#/teacher/papers/${createdPaper.id}`; }, 1500);
      }
    } finally {
      busy = false;
      if (!navigating) {
        aiBtn.disabled = false;
        manualBtn.disabled = false;
        activeBtn.textContent = originalText;
      }
    }
  }
}

function jobStageText(job) {
  if (job.status === "pending") return "排队中…";
  const message = job.message || "";
  if (message.includes("答案")) return "AI 正在补全答案…";
  return "AI 正在识别题目…";
}

async function pollJob(jobId, onTick) {
  for (let i = 0; i < 450; i++) {
    const job = await api.get(`/jobs/${jobId}`);
    if (onTick) onTick(job);
    if (["succeeded", "failed", "canceled"].includes(job.status)) return job;
    await sleep(2000);
  }
  return { status: "failed", last_error: "等待任务超时" };
}

async function detail(root, params) {
  const paper = await api.get(`/papers/${params.id}`);
  const questions = paper.questions || [];
  const canPublish = paper.status !== "published" && paper.status !== "archived";
  const publishBtn = canPublish
    ? el("button", { class: "btn btn-primary", type: "button", onclick: publish }, "发布试卷")
    : null;
  const actions = [
    el("a", { class: "btn", href: `#/teacher/papers/${paper.id}/edit` }, "校对 / 编辑题目"),
    publishBtn,
    paper.status === "published"
      ? el("button", { class: "btn", type: "button", onclick: () => setStatus("archive") }, "归档")
      : null,
    paper.status === "archived"
      ? el("button", { class: "btn", type: "button", onclick: () => setStatus("restore") }, "恢复")
      : null,
    el("button", { class: "btn btn-danger", type: "button", onclick: remove }, "删除"),
  ].filter(Boolean);

  const meta = el("section", { class: "card" },
    el("div", { class: "card-body grid four" },
      metaItem("科目", `${paper.subject?.name || "—"} · ${paper.stage === "junior" ? "初中" : "高中"}`),
      metaItem("题数 / 总分", `${paper.question_count} 题 / ${paper.total_score} 分`),
      metaItem("限时", paper.duration_min ? `${paper.duration_min} 分钟` : "不限时"),
      metaItem("作答", `${paper.stats?.attempts || 0} 份 / 已阅 ${paper.stats?.graded || 0} 份 / 均分 ${Number(paper.stats?.avg_score || 0).toFixed(1)}`)
    )
  );

  const statusRow = el("div", { class: "badges" },
    badge(statusText(paper.status), statusTone(paper.status)),
    badge(parseText(paper.parse_status), parseTone(paper.parse_status)),
    paper.ai_grading ? badge("AI 阅卷", "blue") : badge("人工阅卷", "gray"),
    paper.show_answer ? badge("展示答案", "gray") : null
  );

  const uploader = createUploader({ hint: "点击选择，或截图后直接 Ctrl+V 粘贴；可反复追加" });
  let reparseBusy = false;
  const reparseBtn = el("button", { class: "btn btn-primary", type: "button", onclick: reparse }, "上传并识别");
  const isPublished = paper.status === "published";
  const reparseMode = select([
    { value: "append", label: "追加到末尾（保留已有题目和文件）" },
    ...(isPublished ? [] : [{ value: "replace", label: "替换全部（清空已有题目后重新识别）" }]),
  ], "append");
  const parseProgress = el("div", { class: "parse-progress hidden" },
    el("div", { class: "progress" }, el("div", { class: "progress-bar", style: { width: "0%" } })),
    el("p", { class: "muted" }, "")
  );
  const parseBox = el("section", { class: "card" },
    el("div", { class: "card-head" },
      el("h2", null, isPublished ? "追加题目" : "批量上传 / 增量识别"),
      el("span", { class: "muted small" }, `已有 ${questions.length} 题`)
    ),
    el("div", { class: "card-body stack" },
      paper.parse_error ? el("div", { class: "error-box" }, paper.parse_error) : null,
      isPublished ? el("div", { class: "notice" }, "已发布试卷仅支持追加题目，不能替换全部；追加后仍保持发布状态。") : null,
      el("p", { class: "muted small" }, "同一套试卷支持一次选择多张图片或 PDF，也支持分多次上传：追加模式只识别本次新文件，原有题目与文件保持不变。"),
      uploader.node,
      el("div", { class: "grid two" },
        field("识别方式", reparseMode),
        el("div", { class: "actions", style: { alignItems: "flex-end" } }, reparseBtn)
      ),
      parseProgress
    )
  );

  const qCard = el("section", { class: "card" },
    el("div", { class: "card-head" }, el("h2", null, "题目预览"), el("span", { class: "muted small" }, `共 ${questions.length} 题`)),
    el("div", { class: "card-body question-preview" },
      questions.length === 0
        ? el("p", { class: "muted" }, "还没有题目，去上传题目截图识别。")
        : el("div", { class: "stack" }, ...questions.map((q) => questionPreview(q)))
    )
  );

  root.append(
    pageHeader(paper.title, `创建于 ${fmtDate(paper.created_at, true)}`, actions),
    statusRow, meta, parseBox, qCard
  );

  questions.forEach((q) => {
    const target = root.querySelector(`[data-stem="${q.id}"]`);
    if (target) renderRich(target, q.stem);
  });

  async function publish() {
    if (!publishBtn || publishBtn.disabled) return;
    publishBtn.disabled = true;
    publishBtn.textContent = "发布中…";
    try {
      await api.post(`/papers/${paper.id}/publish`);
      toast("试卷已发布", "success", 4000);
      setTimeout(() => location.reload(), 1000);
    } catch (err) {
      publishBtn.disabled = false;
      publishBtn.textContent = "发布试卷";
      const problems = err.data?.problems;
      if (problems?.length) {
        modal({
          title: "暂时无法发布",
          content: el("ul", { class: "problem-list" }, ...problems.map((p) => el("li", null, p))),
          actions: [{ text: "去校对", primary: true, onClick: () => { location.hash = `#/teacher/papers/${paper.id}/edit`; } }],
        });
      } else {
        toast(errorText(err), "error", 8000);
      }
    }
  }

  async function setStatus(action) {
    try {
      await api.post(`/papers/${paper.id}/${action}`);
      toast("已更新", "success");
      location.reload();
    } catch (err) {
      toast(errorText(err), "error");
    }
  }

  async function remove() {
    if (!(await confirmDialog("确定删除这份试卷？", { danger: true, confirmText: "删除" }))) return;
    try {
      await api.del(`/papers/${paper.id}`);
      toast("已删除", "success");
      location.hash = "#/teacher/papers";
    } catch (err) {
      toast(errorText(err), "error");
    }
  }

  async function reparse() {
    if (reparseBusy) return;
    const picked = uploader.getFiles();
    if (!picked.length) return toast("请选择或粘贴文件", "warn");
    const mode = reparseMode.value;
    if (mode === "replace") {
      const ok = await confirmDialog("替换识别会清空当前试卷的全部题目后重新生成，确定继续？", {
        danger: true,
        confirmText: "替换识别",
      });
      if (!ok) return;
    }
    const progressText = parseProgress.querySelector("p");
    const progressBar = parseProgress.querySelector(".progress-bar");
    const originalText = reparseBtn.textContent;
    const totalMB = (picked.reduce((sum, f) => sum + f.size, 0) / 1024 / 1024).toFixed(1);
    reparseBusy = true;
    reparseBtn.disabled = true;
    reparseBtn.textContent = "正在上传…";
    parseProgress.classList.remove("hidden");
    progressBar.style.width = "8%";
    progressText.textContent = `正在上传 ${picked.length} 个文件（${totalMB} MB）…`;

    const fd = new FormData();
    for (const f of picked) fd.append("files", f);
    fd.append("mode", mode);
    try {
      const res = await api.form(`/papers/${paper.id}/parse`, "POST", fd);
      reparseBtn.textContent = "AI 正在识别…";
      progressText.textContent = "文件已上传，AI 正在识别题目…";
      progressBar.style.width = "10%";
      const job = await pollJob(res.job_id, (j) => {
        const pct = Math.max(10, j.progress || 0);
        progressBar.style.width = `${pct}%`;
        progressText.textContent = `${j.message || "AI 识别中"}（${pct}%）`;
        reparseBtn.textContent = jobStageText(j);
      });
      if (job.status === "succeeded") {
        reparseBtn.textContent = "识别完成";
        toast(job.message || (mode === "append" ? "增量识别完成，题目已追加" : "识别完成，请校对题目"), "success", 6000);
        setTimeout(() => location.reload(), 1500);
      } else {
        reparseBtn.textContent = "识别失败";
        toast(job.last_error || "识别失败", "error", 8000);
        setTimeout(() => location.reload(), 1500);
      }
    } catch (err) {
      reparseBusy = false;
      reparseBtn.disabled = false;
      reparseBtn.textContent = originalText;
      progressText.textContent = `上传或识别失败：${errorText(err)}`;
      toast(errorText(err), "error", 8000);
    }
  }
}

function metaItem(label, value) {
  return el("div", { class: "meta-item" }, el("span", { class: "muted small" }, label), el("strong", null, value));
}

function questionPreview(q) {
  return el("div", { class: "question-preview-item" },
    el("div", { class: "question-head" },
      el("strong", null, `第 ${q.seq} 题`),
      el("span", { class: "muted small" }, `${typeText(q.type)} · ${q.score} 分 · ${q.grading_mode === "manual" ? "人工阅卷" : q.type === "short" ? "AI 阅卷" : "自动判分"}`),
      q.need_manual ? badge("待补答案", "amber") : null
    ),
    el("div", { class: "rich", dataset: { stem: q.id } }, el("span", { class: "muted" }, q.stem)),
    questionFigure(q.source_image),
    q.options?.length ? optionList(q.options) : null,
    el("div", { class: "answer-line muted small" }, "答案：", richInline((q.answer || []).join("；") || "—"))
  );
}

function typeText(t) {
  return { single: "单选", multiple: "多选", judge: "判断", fill: "填空", short: "解答" }[t] || t;
}

function optionList(options) {
  return el("div", { class: "options" },
    ...options.map((o) => el("div", { class: "option-line" }, `${o.key}. `, richInline(o.text)))
  );
}

async function edit(root, params) {
  const paper = await api.get(`/papers/${params.id}`);
  let questions = paper.questions || [];
  const listNode = el("div", { class: "stack" });

  root.append(
    pageHeader(`校对：${paper.title}`, "检查题干、答案与分值；低置信度题目请重点核对", [
      el("a", { class: "btn", href: `#/teacher/papers/${paper.id}` }, "返回详情"),
      el("a", { class: "btn btn-primary", href: `#/teacher/papers/${paper.id}` }, "添加题目"),
    ]),
    el("div", { class: "notice" }, `共 ${questions.length} 题，总分 ${questions.reduce((s, q) => s + Number(q.score || 0), 0)} 分`),
    listNode
  );

  renderList();

  function renderList() {
    listNode.replaceChildren(...questions.map((q, idx) => questionEditorCard(q, idx)));
    questions.forEach((q) => {
      const target = listNode.querySelector(`[data-edit-stem="${q.id}"]`);
      if (target) renderRich(target, q.stem);
    });
  }

  function questionEditorCard(q, idx) {
    const aiBtn = el("button", {
      class: "btn btn-sm",
      type: "button",
      title: "AI 重新作答并复核当前答案",
      onclick: () => aiAnswer(q, aiBtn),
    }, "AI 补答案");
    return el("section", { class: "card question-edit" },
      el("div", { class: "card-head" },
        el("div", { class: "question-head" },
          el("h2", null, `第 ${q.seq} 题`),
          badge(typeText(q.type), "gray"),
          badge(`${q.score} 分`, "blue"),
          q.ai_confidence ? badge(`置信 ${Math.round(q.ai_confidence * 100)}%`, q.ai_confidence < 0.8 ? "amber" : "gray") : null,
          q.need_manual ? badge("待补答案", "amber") : null
        ),
        el("div", { class: "actions" },
          el("button", { class: "icon-btn", type: "button", title: "上移", disabled: idx === 0, onclick: () => move(idx, -1) }, "↑"),
          el("button", { class: "icon-btn", type: "button", title: "下移", disabled: idx === questions.length - 1, onclick: () => move(idx, 1) }, "↓"),
          aiBtn,
          el("button", { class: "btn btn-sm", type: "button", onclick: () => openQuestionForm(q) }, "编辑"),
          el("button", { class: "btn btn-sm btn-danger", type: "button", onclick: () => removeQuestion(q) }, "删除")
        )
      ),
      el("div", { class: "card-body" },
        el("div", { class: "rich stem-preview", dataset: { editStem: q.id } }, q.stem),
        questionFigure(q.source_image),
        q.options?.length ? optionList(q.options) : null,
        el("div", { class: "muted small" },
          "答案：", richInline((q.answer || []).join("；") || "—"),
          ` · 难度：${q.difficulty} · 知识点：${(q.knowledge || []).join("、") || "—"}`
        )
      )
    );
  }

  async function aiAnswer(q, btn) {
    if (btn.disabled) return;
    btn.disabled = true;
    btn.textContent = "AI 作答中…";
    toast("AI 正在作答，请耐心等待…", "info", 12000);
    try {
      const data = await api.post(`/papers/${paper.id}/questions/${q.id}/ai-answer`, {});
      const index = questions.findIndex((item) => item.id === q.id);
      if (index >= 0) questions[index] = data.question;
      renderList();
      if (data.changed) {
        toast(`AI 已修正答案：${answerText(data.previous_answer)} → ${answerText(data.question?.answer)}`, "success", 8000);
      } else {
        toast(data.feedback || "AI 复核完成，答案正确", "success", 7000);
      }
    } catch (err) {
      btn.disabled = false;
      btn.textContent = "AI 补答案";
      toast(errorText(err), "error", 8000);
    }
  }

  function answerText(value) {
    return (value || []).join("；") || "（空）";
  }

  async function move(index, delta) {
    const next = [...questions];
    const [item] = next.splice(index, 1);
    next.splice(index + delta, 0, item);
    try {
      await api.put(`/papers/${paper.id}/questions/order`, { question_ids: next.map((q) => q.id) });
      questions = next.map((q, i) => ({ ...q, seq: i + 1 }));
      renderList();
    } catch (err) {
      toast(errorText(err), "error");
    }
  }

  async function removeQuestion(q) {
    const ok = await confirmDialog(
      `删除第 ${q.seq} 题？该题在未提交答卷中的作答记录也会一并清除。`,
      { danger: true, confirmText: "删除" }
    );
    if (!ok) return;
    try {
      await api.del(`/papers/${paper.id}/questions/${q.id}`);
      const data = await api.get(`/papers/${paper.id}`);
      questions = data.questions || [];
      renderList();
      toast("已删除", "success");
    } catch (err) {
      showQuestionDeleteError(err);
    }
  }

  function showQuestionDeleteError(err) {
    const blocked = err?.status === 422 || err?.status === 409;
    modal({
      title: blocked ? "暂时无法删除该题" : "删除失败",
      width: 440,
      content: el("div", { class: "stack" },
        el("p", null, blocked
          ? "这道题已有学生提交的答卷，为避免影响成绩，系统没有删除。"
          : "删除时遇到问题，请稍后重试。"),
        blocked
          ? el("p", { class: "muted small" }, "可以先编辑题目，或将试卷归档后再处理。")
          : el("p", { class: "muted small" }, "如果多次失败，请刷新页面后再试。")
      ),
      actions: [{ text: "知道了", primary: true }],
    });
  }

  function openQuestionForm(q) {
    if (!q) return;
    const type = select([
      { value: "single", label: "单选题" },
      { value: "multiple", label: "多选题" },
      { value: "judge", label: "判断题" },
      { value: "fill", label: "填空题" },
      { value: "short", label: "解答题" },
    ], q?.type || "single");
    const score = el("input", { class: "input", type: "number", step: "0.5", min: "0.5", value: q?.score ?? 5 });
    const difficulty = select([{ value: "easy", label: "简单" }, { value: "medium", label: "中等" }, { value: "hard", label: "困难" }], q?.difficulty || "medium");
    const grading = select([
      { value: "auto", label: "自动（客观题本地判分 / 主观题 AI）" },
      { value: "ai", label: "AI 阅卷" },
      { value: "manual", label: "人工阅卷" },
    ], q?.grading_mode || "auto");
    const knowledge = el("input", { class: "input", value: (q?.knowledge || []).join("，"), placeholder: "多个知识点用逗号分隔" });
    const stemInput = createMathInput({ value: q?.stem || "", rows: 5, placeholder: "题干，支持 $公式$" });
    const options = el("textarea", { class: "input", rows: 4, value: (q?.options || []).map((o) => `${o.key}. ${o.text}`).join("\n"), placeholder: "每行一个选项，如：A. 2 或 3" });
    const answer = el("textarea", { class: "input", rows: 4, value: (q?.answer || []).join("\n"), placeholder: "选择题填选项字母；填空题每行一个空；解答题填参考答案或留空设为人工阅卷" });
    const analysisInput = createMathInput({ value: q?.analysis || "", rows: 3, placeholder: "解析（可选）" });

    const content = el("div", { class: "stack" },
      el("div", { class: "grid three" }, field("题型", type), field("分值", score), field("难度", difficulty)),
      field("阅卷方式", grading),
      field("知识点", knowledge),
      field("题干", stemInput.node),
      field("选项（选择题必填）", options),
      field("答案", answer),
      field("解析", analysisInput.node)
    );

    modal({
      title: `编辑第 ${q.seq} 题`,
      width: 820,
      content,
      actions: [
        { text: "取消" },
        {
          text: "保存",
          primary: true,
          onClick: async (close) => {
            const payload = {
              type: type.value,
              stem: stemInput.getValue().trim(),
              options: parseOptions(options.value),
              answer: parseAnswers(answer.value),
              score: Number(score.value) || 0,
              analysis: analysisInput.getValue().trim(),
              knowledge: knowledge.value.split(/[,，、]/).map((s) => s.trim()).filter(Boolean),
              difficulty: difficulty.value,
              grading_mode: grading.value,
            };
            if (!payload.stem) {
              toast("题干不能为空", "warn");
              stemInput.focus();
              return false;
            }
            try {
              await api.put(`/papers/${paper.id}/questions/${q.id}`, payload);
              const data = await api.get(`/papers/${paper.id}`);
              questions = data.questions || [];
              renderList();
              toast("已保存", "success");
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
}

function parseOptions(text) {
  return text.split("\n").map((line) => line.trim()).filter(Boolean).map((line) => {
    const m = line.match(/^([A-Za-z])[.、:：|]\s*(.*)$/);
    if (m) return { key: m[1].toUpperCase(), text: m[2] };
    return { key: "", text: line };
  }).filter((o) => o.key);
}

function parseAnswers(text) {
  return text.split("\n").map((s) => s.trim()).filter(Boolean);
}
