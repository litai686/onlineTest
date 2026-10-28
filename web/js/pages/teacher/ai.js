import { api } from "../../api.js";
import { el, pageHeader, badge, modal, field, select, toast, errorText, confirmDialog } from "../../utils.js";

const roleLabels = { vision: "试卷识别", grading: "主观题阅卷", chat: "通用 / 连通性", default: "默认兜底" };
let currentRoles = {};

export async function render(root) {
  const body = el("div");
  const header = pageHeader("AI 模型配置", "兼容 OpenAI 接口的服务均可接入，密钥加密存储", [
    el("button", { class: "btn btn-primary", type: "button", onclick: () => openForm() }, "添加模型"),
  ]);
  root.append(header, body);

  async function load() {
    body.replaceChildren(el("p", { class: "muted" }, "加载中…"));
    try {
      const [data, calls] = await Promise.all([
        api.get("/ai/providers"),
        api.get("/ai/calls", { page: 1, page_size: 12 }),
      ]);
      const providers = data.items || [];
      const callItems = calls.items || [];
      currentRoles = data.roles || {};
      const rolePanel = el("section", { class: "card" },
        el("div", { class: "card-head" }, el("h2", null, "角色绑定")),
        el("div", { class: "card-body grid two" },
          ...Object.entries(roleLabels).map(([role, label]) => boundRole(role, label, data.roles, providers))
        )
      );

      const list = el("section", { class: "card" },
        el("div", { class: "card-head" }, el("h2", null, "模型列表")),
        el("div", { class: "card-body" },
          providers.length === 0
            ? el("p", { class: "muted" }, "还没有配置模型。点击右上角添加，例如 DeepSeek、通义千问、OpenAI 或本地 Ollama。")
            : el("div", { class: "provider-list" },
                ...providers.map((p) => providerCard(p, data.roles))
              )
        )
      );

      const callCard = el("section", { class: "card" },
        el("div", { class: "card-head" }, el("h2", null, "最近调用")),
        el("div", { class: "card-body" },
          callItems.length === 0
            ? el("p", { class: "muted" }, "暂无调用记录")
            : el("div", { class: "table-wrap" },
                el("table", { class: "table" },
                  el("thead", null, el("tr", null,
                    el("th", null, "时间"), el("th", null, "用途"), el("th", null, "模型"),
                    el("th", null, "状态"), el("th", null, "耗时"), el("th", null, "Tokens")
                  )),
                  el("tbody", null, ...callItems.map((c) =>
                    el("tr", null,
                      el("td", null, new Date(c.created_at).toLocaleString()),
                      el("td", null, c.purpose || "—"),
                      el("td", null, c.model || c.provider_name || "—"),
                      el("td", null, badge(c.status, c.status === "ok" ? "green" : c.status === "retry" ? "amber" : "red")),
                      el("td", null, `${c.latency_ms} ms`),
                      el("td", null, `${c.prompt_tokens + c.completion_tokens}`)
                    )
                  ))
                )
              )
        )
      );

      body.replaceChildren(rolePanel, list, callCard);
    } catch (err) {
      body.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
    }
  }

  function boundRole(role, label, roles, providers) {
    const selectNode = select(
      [{ value: "", label: "未绑定" }, ...providers.filter((p) => p.status === 1).map((p) => ({ value: String(p.id), label: `${p.name}（${p.model}）` }))],
      roles[role] || ""
    );
    return el("div", { class: "role-row" },
      el("div", null, el("strong", null, label), el("small", { class: "muted" }, `ai.${role === "default" ? "default" : `role.${role}`}`)),
      selectNode,
      el("button", {
        class: "btn btn-sm",
        type: "button",
        onclick: async () => {
          try {
            await api.post("/ai/roles", { role, provider_id: Number(selectNode.value) || 0 });
            toast("角色绑定已更新", "success");
            load();
          } catch (err) {
            toast(errorText(err), "error");
          }
        },
      }, "保存")
    );
  }

  function providerCard(p, roles) {
    const bound = Object.entries(roles).filter(([, id]) => String(id) === String(p.id)).map(([r]) => roleLabels[r]);
    return el("div", { class: "provider" },
      el("div", { class: "provider-head" },
        el("div", null,
          el("strong", null, p.name),
          el("div", { class: "muted small" }, `${p.model} · ${p.base_url}`)
        ),
        el("div", { class: "provider-badges" },
          p.supports_vision ? badge("视觉", "blue") : null,
          p.thinking_mode ? badge(`思考:${({ none: "关", low: "低", high: "高", max: "最高" })[p.thinking_mode] || p.thinking_mode}`, "amber") : null,
          badge(p.status === 1 ? "启用" : "禁用", p.status === 1 ? "green" : "gray"),
          ...bound.map((b) => badge(b, "amber"))
        )
      ),
      el("div", { class: "provider-meta muted small" },
        `密钥 ${p.api_key_hint || "未配置"} · 温度 ${p.temperature} · 超时 ${p.timeout_sec}s · 并发 ${p.concurrency}`
      ),
      el("div", { class: "actions" },
        el("button", {
          class: "btn btn-sm",
          type: "button",
          onclick: async (e) => {
            const btn = e.currentTarget;
            btn.disabled = true;
            btn.textContent = "测试中…";
            try {
              const res = await api.post(`/ai/providers/${p.id}/test`, { probe_vision: false });
              toast(`连接成功（${res.latency_ms}ms）：${res.reply}`, "success", 6000);
            } catch (err) {
              toast(errorText(err), "error", 8000);
            } finally {
              btn.disabled = false;
              btn.textContent = "测试连接";
            }
          },
        }, "测试连接"),
        el("button", { class: "btn btn-sm", type: "button", onclick: () => openForm(p) }, "编辑"),
        el("button", { class: "btn btn-sm btn-danger", type: "button", onclick: () => remove(p) }, "删除")
      )
    );
  }

  function openForm(p = null) {
    const isNew = !p?.id;
    const name = el("input", { class: "input", value: p?.name || "", placeholder: "如：DeepSeek V3" });
    const base = el("input", { class: "input", value: p?.base_url || "", placeholder: "https://api.deepseek.com/v1" });
    const model = el("input", { class: "input", value: p?.model || "", placeholder: "deepseek-chat" });
    const key = el("input", { class: "input", type: "password", placeholder: isNew ? "sk-..." : "留空表示不修改" });
    const vision = el("input", { type: "checkbox", checked: Boolean(p?.supports_vision) });
    const temperature = el("input", { class: "input", type: "number", step: "0.1", min: "0", max: "2", value: p?.temperature ?? 0.2 });
    const timeout = el("input", { class: "input", type: "number", value: p?.timeout_sec ?? 120 });
    const extraHeaders = el("textarea", {
      class: "input",
      rows: 2,
      placeholder: '例如 {"x-opencode-session":"..."}（可选）',
      value: JSON.stringify(p?.extra_headers || {}),
    });
    const concurrency = el("input", { class: "input", type: "number", value: p?.concurrency ?? 2 });
    const thinking = select([
      { value: "", label: "服务商默认" },
      { value: "none", label: "关闭思考" },
      { value: "low", label: "低" },
      { value: "high", label: "高" },
      { value: "max", label: "最高" },
    ], p?.thinking_mode || "");
    const roles = new Set(Object.entries(rolesCurrent(p)).filter(([, v]) => v).map(([k]) => k));
    const roleBoxes = Object.entries(roleLabels).map(([role, label]) =>
      el("label", { class: "check" }, el("input", {
        type: "checkbox",
        checked: roles.has(role),
        dataset: { role },
        onchange: (e) => { if (e.target.checked) roles.add(role); else roles.delete(role); },
      }), label)
    );
    const enabled = select([{ value: "1", label: "启用" }, { value: "0", label: "禁用" }], String(p?.status ?? 1));
    modal({
      title: isNew ? "添加模型" : "编辑模型",
      width: 680,
      content: el("div", { class: "stack" },
        el("div", { class: "grid two" }, field("名称", name), field("模型名", model)),
        field("Base URL", base, "需以 /v1 结尾（或直接填写完整的 /chat/completions 地址）"),
        field("API Key", key, "使用 AES-256-GCM 加密存储；列表只显示末 4 位"),
        field("额外请求头（JSON，可选）", extraHeaders, "OpenCode 等网关需要的自定义请求头；自动补充 User-Agent 与 x-opencode-session"),
        el("div", { class: "grid three" },
          field("温度", temperature), field("超时（秒）", timeout), field("思考模式", thinking, "映射为 reasoning_effort；部分服务商仅支持 low / high")
        ),
        el("div", { class: "grid two" },
          field("并发上限", concurrency),
          el("label", { class: "check" }, vision, "支持图片（视觉）")
        ),
        field("状态", enabled),
        el("div", { class: "field" }, el("span", { class: "field-label" }, "角色绑定"), el("div", { class: "checks" }, ...roleBoxes))
      ),
      actions: [
        { text: "取消" },
        {
          text: "保存",
          primary: true,
          onClick: async (close) => {
            if (!name.value.trim() || !base.value.trim() || !model.value.trim()) {
              toast("名称、Base URL、模型名不能为空", "warn");
              return false;
            }
            let headers = {};
            try {
              const rawHeaders = extraHeaders.value.trim();
              headers = rawHeaders ? JSON.parse(rawHeaders) : {};
              if (!headers || typeof headers !== "object" || Array.isArray(headers)) throw new Error("not object");
            } catch {
              toast("额外请求头必须是 JSON 对象", "warn");
              extraHeaders.focus();
              return false;
            }
            const payload = {
              name: name.value.trim(),
              base_url: base.value.trim(),
              model: model.value.trim(),
              supports_vision: vision.checked,
              temperature: Number(temperature.value),
              timeout_sec: Number(timeout.value),
              concurrency: Number(concurrency.value),
              thinking_mode: thinking.value,
              status: Number(enabled.value),
              extra_headers: headers,
              set_roles: Array.from(roles),
            };
            if (key.value) payload.api_key = key.value;
            try {
              const previous = rolesCurrent(p);
              const nextRoles = new Set(roles);
              if (isNew) {
                await api.post("/ai/providers", payload);
              } else {
                await api.put(`/ai/providers/${p.id}`, payload);
                for (const role of Object.keys(previous)) {
                  if (!nextRoles.has(role)) {
                    await api.post("/ai/roles", { role, provider_id: 0 });
                  }
                }
              }
              toast("已保存", "success");
              close();
              load();
            } catch (err) {
              toast(errorText(err), "error");
              return false;
            }
          },
        },
      ],
    });
  }

  function rolesCurrent(p) {
    if (!p?.id) return {};
    return Object.fromEntries(Object.entries(currentRoles).filter(([, id]) => String(id) === String(p.id)));
  }

  async function remove(p) {
    if (!(await confirmDialog(`确定删除模型「${p.name}」？绑定的角色会自动解绑。`, { danger: true, confirmText: "删除" }))) return;
    try {
      await api.del(`/ai/providers/${p.id}`);
      toast("已删除", "success");
      load();
    } catch (err) {
      toast(errorText(err), "error");
    }
  }

  load();
}
