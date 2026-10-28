import { el, toast } from "../utils.js";

export const IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"];
const ACCEPTED = [...IMAGE_TYPES, "application/pdf"];

function formatSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function createUploader({
  maxFiles = 30,
  maxFileMB = 20,
  maxTotalMB = 200,
  accept = ACCEPTED,
  title = "上传试卷文件",
  formats = "JPG / PNG / WebP / PDF",
  hint = "点击选择，或直接 Ctrl+V 粘贴截图",
} = {}) {
  const allowedTypes = Array.isArray(accept) && accept.length ? accept : ACCEPTED;
  const totalBytes = () => files.reduce((sum, f) => sum + f.size, 0);
  const items = [];
  const files = [];
  const fileInput = el("input", {
    type: "file",
    multiple: true,
    accept: allowedTypes.join(","),
    class: "hidden",
  });
  const list = el("div", { class: "upload-list" });
  const count = el("span", { class: "muted small" }, "未选择文件");
  const zone = el("div", { class: "upload-zone", tabindex: "0" },
    el("strong", null, title),
    el("span", { class: "muted small" }, hint),
    el("span", { class: "muted small" }, `支持 ${formats}，单次最多 ${maxFiles} 个，单文件 ≤${maxFileMB}MB`)
  );

  function validate(file) {
    if (!allowedTypes.includes(file.type)) {
      toast(`不支持的格式：${file.name || file.type}`, "warn");
      return false;
    }
    if (file.size > maxFileMB * 1024 * 1024) {
      toast(`${file.name || "文件"} 超过 ${maxFileMB}MB`, "warn");
      return false;
    }
    return true;
  }

  function addFiles(incoming) {
    const incomingList = Array.from(incoming || []);
    if (!incomingList.length) return;
    for (const raw of incomingList) {
      if (files.length >= maxFiles) {
        toast(`单次最多 ${maxFiles} 个文件，请分批上传`, "warn");
        break;
      }
      let file = raw;
      if (!file.name) {
        const ext = file.type === "application/pdf" ? "pdf" : file.type === "image/png" ? "png" : "jpg";
        file = new File([raw], `screenshot-${Date.now()}.${ext}`, { type: raw.type });
      }
      if (!validate(file)) continue;
      if (totalBytes() + file.size > maxTotalMB * 1024 * 1024) {
        toast(`单次总量不能超过 ${maxTotalMB}MB`, "warn");
        break;
      }
      files.push(file);
      const preview = file.type.startsWith("image/")
        ? el("img", { src: URL.createObjectURL(file), alt: file.name })
        : el("span", { class: "upload-pdf" }, "PDF");
      const remove = el("button", {
        class: "icon-btn",
        type: "button",
        title: "移除",
        onclick: (event) => {
          event.stopPropagation();
          if (preview.tagName === "IMG") URL.revokeObjectURL(preview.src);
          items.splice(items.indexOf(entry), 1);
          files.splice(files.indexOf(file), 1);
          entry.remove();
          updateCount();
        },
      }, "×");
      const entry = el("div", { class: "upload-item" },
        preview,
        el("div", { class: "upload-meta" },
          el("strong", null, file.name),
          el("span", { class: "muted small" }, `${file.type || "未知类型"} · ${formatSize(file.size)}`)
        ),
        remove
      );
      items.push(entry);
      list.append(entry);
    }
    updateCount();
    fileInput.value = "";
  }

  function updateCount() {
    const total = totalBytes();
    count.textContent = files.length
      ? `已选择 ${files.length} 个文件 · 共 ${formatSize(total)}`
      : "未选择文件";
  }

  function onPaste(event) {
    if (!zone.isConnected) {
      document.removeEventListener("paste", onPaste);
      return;
    }
    const target = event.target;
    if (target instanceof Element && target.closest("input, textarea, [contenteditable='true']")) {
      return;
    }
    const clipboard = event.clipboardData;
    if (!clipboard) return;
    const imageFiles = [];
    for (const item of clipboard.items || []) {
      if (item.kind === "file" && item.type.startsWith("image/")) {
        const file = item.getAsFile();
        if (file) imageFiles.push(file);
      }
    }
    if (!imageFiles.length) return;
    event.preventDefault();
    addFiles(imageFiles);
    toast(`已从剪贴板添加 ${imageFiles.length} 张图片`, "success");
  }

  zone.addEventListener("click", () => fileInput.click());
  zone.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      fileInput.click();
    }
  });
  fileInput.addEventListener("change", () => addFiles(fileInput.files));
  zone.addEventListener("dragover", (event) => {
    event.preventDefault();
    zone.classList.add("dragover");
  });
  zone.addEventListener("dragleave", () => zone.classList.remove("dragover"));
  zone.addEventListener("drop", (event) => {
    event.preventDefault();
    zone.classList.remove("dragover");
    if (event.dataTransfer?.files?.length) addFiles(event.dataTransfer.files);
  });
  document.addEventListener("paste", onPaste);
  updateCount();

  return {
    node: el("div", { class: "uploader" }, zone, fileInput, count, list),
    getFiles: () => files.slice(),
    clear: () => {
      items.forEach((item) => {
        const img = item.querySelector("img");
        if (img) URL.revokeObjectURL(img.src);
        item.remove();
      });
      items.length = 0;
      files.length = 0;
      updateCount();
    },
    focus: () => zone.focus(),
  };
}
