import { fetchBlob } from "../api.js";
import { el } from "../utils.js";

const urls = new Map();

function load(fileId) {
  if (!urls.has(fileId)) {
    const pending = fetchBlob(`/api/v1/files/${encodeURIComponent(fileId)}`)
      .then((blob) => URL.createObjectURL(blob))
      .catch((err) => {
        urls.delete(fileId);
        throw err;
      });
    urls.set(fileId, pending);
  }
  return urls.get(fileId);
}

function openLightbox(image, label) {
  const src = image.currentSrc || image.getAttribute("src");
  if (!src) return;
  const onKey = (event) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
    }
  };
  const close = () => {
    window.removeEventListener("keydown", onKey, true);
    overlay.remove();
  };
  const overlay = el("div", { class: "lightbox-overlay", role: "dialog", "aria-label": label, tabindex: "-1" },
    el("div", { class: "lightbox-panel" },
      el("img", { class: "lightbox-img", src, alt: label, title: "点击关闭" }),
      el("button", { class: "icon-btn lightbox-close", type: "button", "aria-label": "关闭", onclick: close }, "×")
    )
  );
  overlay.addEventListener("click", close);
  window.addEventListener("keydown", onKey, true);
  document.body.append(overlay);
  overlay.focus();
}

export function questionFigure(fileId, { label = "题目配图" } = {}) {
  if (!fileId) return null;
  const caption = el("figcaption", { class: "muted small" }, "题目配图加载中…");
  const image = el("img", { class: "question-figure-img", alt: label, loading: "lazy" });
  const figure = el("figure", { class: "question-figure is-loading", dataset: { sourceImage: fileId } },
    image,
    caption
  );
  image.addEventListener("click", () => openLightbox(image, label));
  load(fileId).then((url) => {
    image.src = url;
    figure.classList.remove("is-loading");
    figure.classList.add("is-ready");
    caption.textContent = `${label}（点击查看大图）`;
  }).catch(() => {
    figure.classList.remove("is-loading");
    figure.classList.add("is-failed");
    caption.textContent = "题目配图加载失败";
  });
  return figure;
}
