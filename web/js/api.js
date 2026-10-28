import { store } from "./store.js";

async function request(method, path, { body, form, query } = {}) {
  const headers = {};
  if (store.token) headers.Authorization = `Bearer ${store.token}`;
  let payload;
  if (form) {
    payload = form;
  } else if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }
  let url = `/api/v1${path}`;
  if (query) {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) {
      if (v !== undefined && v !== null && v !== "") params.set(k, v);
    }
    const qs = params.toString();
    if (qs) url += `?${qs}`;
  }
  let res;
  try {
    res = await fetch(url, { method, headers, body: payload });
  } catch (err) {
    throw new Error("网络连接失败，请检查服务是否运行");
  }
  if (res.status === 204) return null;
  const text = await res.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    throw new Error(`服务器返回了非 JSON 响应（${res.status}）`);
  }
  if (!res.ok || (data && data.code !== 0)) {
    if (res.status === 401) {
      store.logout();
      if (location.hash !== "#/login") location.hash = "#/login";
    }
    const err = new Error(data?.message || `请求失败（${res.status}）`);
    err.code = data?.code;
    err.status = res.status;
    err.data = data?.data;
    throw err;
  }
  return data?.data;
}

export const api = {
  get: (path, query) => request("GET", path, { query }),
  post: (path, body) => request("POST", path, { body }),
  put: (path, body) => request("PUT", path, { body }),
  del: (path) => request("DELETE", path),
  upload: (path, formData) => request("POST", path, { form: formData }),
  form: (path, method = "POST", formData) => request(method, path, { form: formData }),
};

export async function fetchBlob(path) {
  const headers = {};
  if (store.token) headers.Authorization = `Bearer ${store.token}`;
  const res = await fetch(path, { headers });
  if (!res.ok) {
    let message = `文件加载失败（${res.status}）`;
    try {
      const data = await res.json();
      if (data?.message) message = data.message;
    } catch {}
    throw new Error(message);
  }
  return res.blob();
}

export async function openProtected(path) {
  const blob = await fetchBlob(path);
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.target = "_blank";
  link.rel = "noopener";
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 60_000);
}
