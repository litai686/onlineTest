import { el } from "../utils.js";

let katex = null;
let katexLoading = null;

export async function ensureKatex() {
  if (katex) return katex;
  if (window.katex) {
    katex = window.katex;
    return katex;
  }
  if (!katexLoading) {
    katexLoading = (async () => {
      if (!document.querySelector('link[data-katex]')) {
        const link = document.createElement("link");
        link.rel = "stylesheet";
        link.href = "/vendor/katex/katex.min.css";
        link.dataset.katex = "1";
        document.head.append(link);
      }
      const mod = await import("/vendor/katex/katex.mjs");
      katex = mod.default || mod;
      window.katex = katex;
      return katex;
    })().catch((err) => {
      console.warn("KaTeX 加载失败，公式将降级为纯文本", err);
      return null;
    });
  }
  return katexLoading;
}

export function tokenize(text) {
  const src = String(text ?? "");
  const out = [];
  let buffer = "";
  let i = 0;
  const pushText = () => {
    if (buffer) {
      out.push({ type: "text", value: buffer.replaceAll("\\$", "$") });
      buffer = "";
    }
  };
  while (i < src.length) {
    if (src.startsWith("$$", i)) {
      const end = src.indexOf("$$", i + 2);
      if (end > i + 2) {
        pushText();
        out.push({ type: "display", value: src.slice(i + 2, end) });
        i = end + 2;
        continue;
      }
    }
    if (src[i] === "$" && src[i - 1] !== "\\") {
      const end = src.indexOf("$", i + 1);
      if (end > i + 1 && !src.slice(i + 1, end).includes("\n")) {
        pushText();
        out.push({ type: "inline", value: src.slice(i + 1, end) });
        i = end + 1;
        continue;
      }
    }
    buffer += src[i];
    i += 1;
  }
  pushText();
  return out;
}

function renderMath(value, displayMode) {
  const node = el(displayMode ? "div" : "span", { class: displayMode ? "math-block" : "math-inline" });
  if (katex && typeof katex.render === "function") {
    try {
      katex.render(value, node, {
        displayMode,
        throwOnError: false,
        trust: false,
        strict: "ignore",
        maxExpand: 1000,
      });
      return node;
    } catch {
      /* fall through */
    }
  }
  node.className = "math-fallback";
  node.textContent = displayMode ? `$$${value}$$` : `$${value}$`;
  return node;
}

export async function renderRich(target, text, { block = true } = {}) {
  await ensureKatex();
  target.replaceChildren();
  const tokens = tokenize(text);
  const frag = document.createDocumentFragment();
  for (const token of tokens) {
    if (token.type === "text") {
      const parts = token.value.split("\n");
      parts.forEach((part, idx) => {
        if (part) frag.append(document.createTextNode(part));
        if (idx < parts.length - 1) frag.append(el("br"));
      });
    } else if (token.type === "inline") {
      frag.append(renderMath(token.value, false));
    } else {
      const math = renderMath(token.value, true);
      if (block) frag.append(math);
      else frag.append(el("span", null, math));
    }
  }
  target.append(frag);
  return target;
}

export async function renderRichNode(text, options) {
  const node = el("div", { class: "rich" });
  await renderRich(node, text, options);
  return node;
}

export function stripMathDelimiters(value) {
  let s = String(value ?? "").trim();
  if (s.startsWith("$$") && s.endsWith("$$") && s.length > 4) s = s.slice(2, -2);
  else if (s.startsWith("$") && s.endsWith("$") && s.length > 2) s = s.slice(1, -1);
  return s.trim();
}
