import { el, modal, debounce, toast } from "../utils.js";
import { ensureKatex, renderRich, stripMathDelimiters } from "./math-render.js";

const templates = [
  { label: "分式", insert: "\\frac{}{}" },
  { label: "根号", insert: "\\sqrt{}" },
  { label: "幂", insert: "^{}" },
  { label: "下标", insert: "_{}" },
  { label: "不等号", insert: "\\leq " },
  { label: "π", insert: "\\pi " },
  { label: "角度", insert: "^{\\circ}" },
  { label: "方程组", insert: "\\begin{cases}\n  \n\\end{cases}" },
  { label: "求和", insert: "\\sum_{i=1}^{n} " },
  { label: "积分", insert: "\\int_{a}^{b} " },
];

let mathlivePromise = null;

async function loadMathLive() {
  if (customElements.get("math-field")) return true;
  if (!mathlivePromise) {
    mathlivePromise = (async () => {
      if (!document.querySelector('link[data-mathlive]')) {
        const link = document.createElement("link");
        link.rel = "stylesheet";
        link.href = "/vendor/mathlive/mathlive-fonts.css";
        link.dataset.mathlive = "1";
        document.head.append(link);
      }
      await import("/vendor/mathlive/mathlive.min.mjs");
      await customElements.whenDefined("math-field");
      return true;
    })().catch((err) => {
      console.warn("MathLive 加载失败，使用 LaTeX 文本输入", err);
      return false;
    });
  }
  return mathlivePromise;
}

function insertAtCursor(textarea, text) {
  const start = textarea.selectionStart ?? textarea.value.length;
  const end = textarea.selectionEnd ?? start;
  const value = textarea.value;
  textarea.value = value.slice(0, start) + text + value.slice(end);
  textarea.selectionStart = textarea.selectionEnd = start + text.length;
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
  textarea.focus();
}

export async function editFormula(initial = "") {
  const wrapper = el("div", { class: "formula-editor" });
  let getValue = () => "";
  const preview = el("div", { class: "math-preview" });
  const updatePreview = debounce(async () => {
    await renderRich(preview, `$${getValue()}$`);
  }, 200);

  const ok = await loadMathLive();
  if (ok) {
    const field = document.createElement("math-field");
    field.value = stripMathDelimiters(initial);
    field.setAttribute("virtual-keyboard-mode", "auto");
    field.style.width = "100%";
    field.style.fontSize = "1.3rem";
    field.style.minHeight = "64px";
    field.addEventListener("input", () => {
      getValue = () => field.value;
      updatePreview();
    });
    wrapper.append(field);
    getValue = () => field.value;
    setTimeout(() => field.focus(), 30);
  } else {
    const textarea = el("textarea", {
      class: "input code",
      rows: 3,
      placeholder: "输入 LaTeX，例如 \\frac{1}{2}",
      value: stripMathDelimiters(initial),
    });
    textarea.addEventListener("input", () => {
      getValue = () => textarea.value;
      updatePreview();
    });
    wrapper.append(textarea);
    getValue = () => textarea.value;
    setTimeout(() => textarea.focus(), 30);
  }
  wrapper.append(el("div", { class: "field-hint" }, "支持虚拟键盘输入；确认后插入为 $...$"), preview);
  updatePreview();

  return new Promise((resolve) => {
    modal({
      title: "编辑公式",
      width: 620,
      content: wrapper,
      actions: [
        { text: "取消", onClick: () => resolve(null) },
        {
          text: "插入",
          primary: true,
          onClick: () => {
            const latex = getValue().trim();
            if (!latex) {
              toast("公式为空", "warn");
              return false;
            }
            resolve(latex);
          },
        },
      ],
      onClose: () => resolve(null),
    });
  });
}

export function createMathInput({ value = "", rows = 5, placeholder = "", onChange = null } = {}) {
  const textarea = el("textarea", {
    class: "input math-text",
    rows,
    placeholder: placeholder || "可直接输入文字；点击「公式」插入数学表达式",
    value,
    spellcheck: "false",
  });
  const preview = el("div", { class: "math-preview hidden" });
  const previewBtn = el("button", { class: "btn btn-sm", type: "button" }, "预览");
  const toolbar = el("div", { class: "math-toolbar" },
    el("button", {
      class: "btn btn-sm btn-primary",
      type: "button",
      onclick: async () => {
        const latex = await editFormula();
        if (latex) insertAtCursor(textarea, `$${latex}$`);
      },
    }, "公式"),
    ...templates.map((t) =>
      el("button", {
        class: "btn btn-sm",
        type: "button",
        title: t.insert,
        onclick: () => insertAtCursor(textarea, t.insert),
      }, t.label)
    ),
    previewBtn
  );
  previewBtn.addEventListener("click", async () => {
    const hidden = preview.classList.toggle("hidden");
    previewBtn.textContent = hidden ? "预览" : "收起预览";
    if (!hidden) await renderRich(preview, textarea.value);
  });
  const refreshPreview = debounce(() => {
    if (!preview.classList.contains("hidden")) renderRich(preview, textarea.value);
  }, 300);
  textarea.addEventListener("input", () => {
    if (onChange) onChange(textarea.value);
    refreshPreview();
  });
  const node = el("div", { class: "math-input" }, toolbar, textarea, preview);
  return {
    node,
    getValue: () => textarea.value,
    setValue: (v) => { textarea.value = v ?? ""; },
    focus: () => textarea.focus(),
    textarea,
  };
}

export async function createSingleMathInput({ value = "", placeholder = "输入答案", onChange = null } = {}) {
  const wrap = el("div", { class: "inline-math" });
  const input = el("input", { class: "input", value, placeholder });
  const btn = el("button", {
    class: "btn btn-sm",
    type: "button",
    onclick: async () => {
      const latex = await editFormula(input.value);
      if (latex) input.value = latex;
      if (onChange) onChange(input.value);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    },
  }, "公式");
  input.addEventListener("input", () => onChange && onChange(input.value));
  wrap.append(input, btn);
  let preview = null;
  wrap.preview = async () => {
    if (!preview) {
      preview = el("div", { class: "math-preview" });
      wrap.append(preview);
    }
    await ensureKatex();
    await renderRich(preview, input.value ? `$${input.value}$` : "");
  };
  return { node: wrap, input, getValue: () => input.value, setValue: (v) => { input.value = v ?? ""; }, preview: wrap.preview };
}
