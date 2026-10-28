export const $ = (selector, root = document) => root.querySelector(selector);
export const $$ = (selector, root = document) => Array.from(root.querySelectorAll(selector));

export function el(tag, props = null, ...children) {
  const node = document.createElement(tag);
  if (props) {
    for (const [key, value] of Object.entries(props)) {
      if (value === null || value === undefined || value === false) continue;
      if (key === "class") node.className = value;
      else if (key === "text") node.textContent = value;
      else if (key === "value") node.value = value;
      else if (key === "checked" || key === "disabled" || key === "selected" || key === "readOnly") node[key] = Boolean(value);
      else if (key === "dataset") Object.assign(node.dataset, value);
      else if (key === "style" && typeof value === "object") Object.assign(node.style, value);
      else if (key.startsWith("on") && typeof value === "function") node.addEventListener(key.slice(2).toLowerCase(), value);
      else node.setAttribute(key, value);
    }
  }
  appendChildren(node, children);
  return node;
}

export function appendChildren(node, children) {
  for (const child of children.flat(Infinity)) {
    if (child === null || child === undefined || child === false) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
}

export function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
}

export function fmtDate(value, withTime = false) {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return String(value);
  const pad = (n) => String(n).padStart(2, "0");
  const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  if (!withTime) return date;
  return `${date} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fmtDuration(seconds) {
  if (!seconds || seconds < 0) return "—";
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m} 分 ${String(s).padStart(2, "0")} 秒`;
}

export function debounce(fn, wait = 300) {
  let timer = null;
  const wrapped = (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), wait);
  };
  wrapped.cancel = () => clearTimeout(timer);
  wrapped.flush = (...args) => {
    clearTimeout(timer);
    fn(...args);
  };
  return wrapped;
}

export function errorText(err) {
  if (!err) return "操作失败";
  return err.message || String(err);
}

export function toast(message, type = "info", timeout = 3200) {
  const root = document.getElementById("toast-root");
  if (!root) return;
  const node = el("div", { class: `toast toast-${type}` }, message);
  root.append(node);
  setTimeout(() => {
    node.classList.add("toast-out");
    setTimeout(() => node.remove(), 220);
  }, timeout);
}

export function loading(text = "加载中…") {
  return el("div", { class: "loading" }, el("span", { class: "spinner" }), text);
}

export function empty(text = "暂无数据") {
  return el("div", { class: "empty" }, text);
}

export function modal({
  title,
  content,
  actions = [],
  width = 560,
  onClose = null,
  closeOnOverlay = false,
  closeOnEsc = false,
}) {
  const root = document.getElementById("modal-root");
  const overlay = el("div", { class: "modal-overlay" });
  const onKeydown = (event) => {
    if (event.key === "Escape") close();
  };
  const close = () => {
    document.removeEventListener("keydown", onKeydown);
    overlay.remove();
    if (onClose) onClose();
  };
  if (closeOnEsc) document.addEventListener("keydown", onKeydown);
  const footer = actions.length
    ? el("div", { class: "modal-actions" },
        ...actions.map((a) =>
          el("button", {
            class: `btn ${a.primary ? "btn-primary" : ""} ${a.danger ? "btn-danger" : ""}`,
            type: "button",
            onclick: async () => {
              if (a.onClick) {
                const result = await a.onClick(close);
                if (result !== false) close();
              } else {
                close();
              }
            },
          }, a.text)
        )
      )
    : null;
  overlay.append(
    el("div", { class: "modal", style: { width: `${width}px` } },
      el("div", { class: "modal-head" },
        el("h3", null, title || ""),
        el("button", { class: "icon-btn", type: "button", "aria-label": "关闭", onclick: close }, "×")
      ),
      el("div", { class: "modal-body" }, content),
      footer
    )
  );
  overlay.addEventListener("click", (e) => {
    if (closeOnOverlay && e.target === overlay) close();
  });
  root.append(overlay);
  return { close, overlay };
}

export function confirmDialog(message, { title = "请确认", confirmText = "确定", danger = false } = {}) {
  return new Promise((resolve) => {
    modal({
      title,
      width: 420,
      content: el("p", { class: "muted" }, message),
      actions: [
        { text: "取消", onClick: () => { resolve(false); } },
        {
          text: confirmText,
          primary: !danger,
          danger,
          onClick: () => { resolve(true); },
        },
      ],
      onClose: () => resolve(false),
    });
  });
}

export function field(label, input, hint = null) {
  return el("label", { class: "field" },
    el("span", { class: "field-label" }, label),
    input,
    hint ? el("span", { class: "field-hint" }, hint) : null
  );
}

export function select(options, value, props = {}) {
  const node = el("select", { class: "input", ...props });
  for (const opt of options) {
    const o = typeof opt === "string" ? { value: opt, label: opt } : opt;
    node.append(el("option", { value: o.value, selected: String(o.value) === String(value ?? "") }, o.label ?? o.value));
  }
  return node;
}

export function table(columns, rows, renderRow, emptyText = "暂无数据") {
  if (!rows || rows.length === 0) return empty(emptyText);
  const head = el("thead", null, el("tr", null, ...columns.map((c) => el("th", null, c))));
  const body = el("tbody", null, ...rows.map((row, i) => renderRow(row, i)));
  return el("div", { class: "table-wrap" }, el("table", { class: "table" }, head, body));
}

export function pageHeader(title, subtitle = null, actions = []) {
  return el("div", { class: "page-head" },
    el("div", null,
      el("h1", null, title),
      subtitle ? el("p", { class: "muted" }, subtitle) : null
    ),
    actions.length ? el("div", { class: "page-actions" }, ...actions) : null
  );
}

export function card(title, ...children) {
  return el("section", { class: "card" },
    title ? el("div", { class: "card-head" }, el("h2", null, title)) : null,
    el("div", { class: "card-body" }, ...children)
  );
}

export function badge(text, tone = "") {
  return el("span", { class: `badge ${tone ? `badge-${tone}` : ""}` }, text);
}

export function nl2br(text) {
  return String(text || "").split("\n").map((line, i) => el("span", null, line, i < String(text).split("\n").length - 1 ? el("br") : null));
}

export function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
