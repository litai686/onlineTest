import { api } from "../../api.js";
import { el, pageHeader, badge, modal, field, select, toast, errorText, confirmDialog } from "../../utils.js";

export async function render(root) {
  const body = el("div");
  root.append(pageHeader("科目管理", "初中 / 高中科目，可按需启用或禁用"), body);

  async function load() {
    body.replaceChildren(el("p", { class: "muted" }, "加载中…"));
    try {
      const data = await api.get("/subjects", { include_disabled: "1" });
      const items = data.items || [];
      const groups = [
        { stage: "junior", label: "初中" },
        { stage: "senior", label: "高中" },
      ];
      body.replaceChildren(
        ...groups.map((g) =>
          el("section", { class: "card" },
            el("div", { class: "card-head" },
              el("h2", null, g.label),
              el("button", { class: "btn btn-sm btn-primary", type: "button", onclick: () => openForm({ stage: g.stage }) }, "新增科目")
            ),
            el("div", { class: "card-body" },
              el("div", { class: "chip-grid" },
                ...items.filter((s) => s.stage === g.stage).map((s) =>
                  el("div", { class: `chip ${s.status === 1 ? "" : "chip-off"}` },
                    el("span", null, s.name),
                    el("small", { class: "muted" }, `${s.paper_count} 份试卷`),
                    el("div", { class: "chip-actions" },
                      el("button", { class: "icon-btn", type: "button", title: "编辑", onclick: () => openForm(s) }, "✎"),
                      el("button", { class: "icon-btn", type: "button", title: "删除", onclick: () => remove(s) }, "🗑")
                    )
                  )
                )
              )
            )
          )
        ),
        el("p", { class: "muted small" }, "删除前请确认该科目下没有试卷；已使用的科目建议禁用。")
      );
    } catch (err) {
      body.replaceChildren(el("div", { class: "error-box" }, errorText(err)));
    }
  }

  function openForm(subject) {
    const isNew = !subject?.id;
    const stage = select([{ value: "junior", label: "初中" }, { value: "senior", label: "高中" }], subject?.stage || "junior");
    const name = el("input", { class: "input", value: subject?.name || "", placeholder: "如：数学" });
    const code = el("input", { class: "input", value: subject?.code || "", placeholder: "可选，如 math" });
    const sort = el("input", { class: "input", type: "number", value: subject?.sort_order ?? 0 });
    const status = select([{ value: "1", label: "启用" }, { value: "0", label: "禁用" }], String(subject?.status ?? 1));
    modal({
      title: isNew ? "新增科目" : "编辑科目",
      content: el("div", { class: "stack" },
        field("学段", stage),
        field("科目名称", name),
        field("代码", code),
        field("排序", sort),
        field("状态", status)
      ),
      actions: [
        { text: "取消" },
        {
          text: "保存",
          primary: true,
          onClick: async (close) => {
            try {
              if (!name.value.trim()) {
                toast("请填写科目名称", "warn");
                name.focus();
                return false;
              }
              const payload = {
                stage: stage.value,
                name: name.value.trim(),
                code: code.value.trim(),
                sort_order: Number(sort.value) || 0,
                status: Number(status.value),
              };
              if (isNew) await api.post("/subjects", payload);
              else await api.put(`/subjects/${subject.id}`, payload);
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

  async function remove(subject) {
    if (!(await confirmDialog(`确定删除科目「${subject.name}」？`, { danger: true, confirmText: "删除" }))) return;
    try {
      await api.del(`/subjects/${subject.id}`);
      toast("已删除", "success");
      load();
    } catch (err) {
      toast(errorText(err), "error");
    }
  }

  load();
}
