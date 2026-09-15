<script setup lang="ts">
import { computed, h, ref } from "vue";
import { NAlert, NButton, NDataTable, NInput, NSwitch } from "naive-ui";
import { api } from "../../api/client";
import { useAppContext } from "../../composables/useAppContext";
import { fmt } from "../../utils/format";
import { tableButton } from "../../utils/table";
import type {
  Route,
  PendingRoutes,
  BatchResult,
  RouteRevision,
} from "../../types";
import type { RouteAction, RouteForm } from "./types";
import RouteDialog from "./RouteDialog.vue";
const { loading, error, notice, run } = useAppContext();
const routes = ref<Route[]>([]),
  search = ref("");
const pending = ref<PendingRoutes>({
  revision: 0,
  applied_revision: -1,
  pending: false,
  history: [],
});
const modal = ref<RouteAction | "">(""),
  selected = ref<Route>(),
  batch = ref<BatchResult[]>([]);
const isEditing = computed(() => !!modal.value);
const filteredRoutes = computed(() =>
  routes.value.filter((r) => (r.cidr + " " + r.remark).includes(search.value)),
);
const button = (text: string, action: () => void, danger = false) =>
  tableButton(text, action, danger, loading.value);
function open(action: RouteAction, route?: Route) {
  error.value = "";
  selected.value = route;
  batch.value = [];
  modal.value = action;
}
async function refresh() {
  routes.value = await api<Route[]>("/routes");
  pending.value = await api<PendingRoutes>("/routes/pending");
}
async function save(form: RouteForm) {
  await run(async () => {
    const path = "/routes/" + selected.value?.id;
    switch (modal.value) {
      case "batch":
        batch.value = await api<BatchResult[]>("/routes/batch", "POST", form);
        await refresh();
        return;
      case "edit-route":
        await api(path, "PUT", {
          remark: form.remark,
          enabled: selected.value?.enabled,
        });
        break;
      case "delete-route":
        await api(path, "DELETE");
        break;
      case "apply":
        const result = await api<{ message: string }>("/routes/apply", "POST", {
          revision: pending.value.revision,
          confirm: true,
        });
        notice.value = result.message;
        break;
    }
    modal.value = "";
    await refresh();
  });
}
const revisionLabels: Record<string, string> = {
  applying: "应用中 / 待恢复",
  applied: "已应用",
  failed: "失败",
  rolled_back: "已回滚",
};
const revisionColumns = [
  { title: "版本", key: "revision" },
  {
    title: "状态",
    key: "status",
    render: (r: RouteRevision) => revisionLabels[r.status] || r.status,
  },
  {
    title: "时间",
    key: "created_at",
    render: (r: RouteRevision) => fmt(r.created_at),
  },
];
defineExpose({ refresh, isEditing });
const routeColumns = [
  { title: "目标 CIDR", key: "cidr", minWidth: 150 },
  { title: "备注", key: "remark", minWidth: 200 },
  {
    title: "状态",
    key: "enabled",
    minWidth: 110,
    render: (r: Route) =>
      h(
        NSwitch,
        {
          value: r.enabled,
          disabled: loading.value,
          "onUpdate:value": (value: boolean) =>
            run(async () => {
              await api("/routes/" + r.id, "PUT", {
                remark: r.remark,
                enabled: value,
              });
              await refresh();
            }),
        },
        { checked: () => "启用", unchecked: () => "禁用" },
      ),
  },
  {
    title: "更新时间",
    key: "updated_at",
    minWidth: 175,
    render: (r: Route) => fmt(r.updated_at),
  },
  {
    title: "操作",
    key: "actions",
    minWidth: 130,
    render: (r: Route) =>
      h("div", { class: "actions" }, [
        button("编辑备注", () => open("edit-route", r)),
        button("删除", () => open("delete-route", r), true),
      ]),
  },
];
</script>
<template>
  <NAlert
    :type="pending.pending ? 'warning' : 'success'"
    :bordered="false"
    class="banner"
    >{{
      pending.pending
        ? "存在未应用变更，保存规则不会立即中断连接。"
        : "当前规则已应用。"
    }}
    当前版本 {{ pending.revision }} / 已应用
    {{ pending.applied_revision < 0 ? "尚未应用" : pending.applied_revision
    }}<NButton
      style="margin-left: 16px"
      :disabled="!pending.pending || loading"
      type="error"
      secondary
      @click="open('apply')"
      >应用配置</NButton
    ></NAlert
  >
  <section class="panel">
    <div class="toolbar">
      <NInput
        v-model:value="search"
        clearable
        placeholder="搜索目标或备注"
      /><NButton type="primary" @click="open('batch')"
        >＋ 新增 / 批量新增</NButton
      >
    </div>
    <NDataTable
      :columns="routeColumns"
      :data="filteredRoutes"
      :loading="loading"
      :scroll-x="800"
      :pagination="{ pageSize: 15 }"
    />
  </section>
  <section v-if="pending.history?.length" class="panel">
    <h3>配置应用记录</h3>
    <NDataTable
      :columns="revisionColumns"
      :data="pending.history.slice().reverse()"
      :pagination="{ pageSize: 5 }"
    />
  </section>
  <RouteDialog
    v-if="modal"
    :action="modal"
    :route="selected"
    :batch="batch"
    @close="modal = ''"
    @submit="save"
  />
</template>
