import { fetchBlob, openProtected } from "../api.js";
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

export function questionFigure(fileId, { label = "题目配图" } = {}) {
  if (!fileId) return null;
  const caption = el("figcaption", { class: "muted small" }, "题目配图加载中…");
  const image = el("img", { class: "question-figure-img", alt: label, loading: "lazy" });
  const figure = el("figure", { class: "question-figure is-loading", dataset: { sourceImage: fileId } },
    image,
    caption
  );
  image.addEventListener("click", () => {
    if (!image.getAttribute("src")) return;
    openProtected(`/api/v1/files/${encodeURIComponent(fileId)}`).catch(() => {});
  });
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
