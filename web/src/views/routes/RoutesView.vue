<script setup lang="ts">
import { computed, h, ref } from "vue";
import { NAlert, NButton, NDataTable, NInput, NSelect, NSwitch, NTabs, NTabPane } from "naive-ui";
import { api } from "../../api/client";
import { useAppContext } from "../../composables/useAppContext";
import { fmt } from "../../utils/format";
import { tableButton } from "../../utils/table";
import type {
  Route,
  RouteProject,
  PendingRoutes,
  BatchResult,
} from "../../types";
import type { RouteAction, RouteForm } from "./types";
import ProjectManager from "./ProjectManager.vue";
import RouteDialog from "./RouteDialog.vue";
const { loading, error, notice, run } = useAppContext();
const projects = ref<RouteProject[]>([]);
const projectFilter = ref("all");

const projectName = (id?: string) => projects.value.find(p => p.id === id)?.name ?? "未分组";
const projectOptions = computed(() => [{label: "全部项目", value: "all"}, {label: "未分组", value: ""}, ...projects.value.map(p => ({label: p.name, value: p.id}))]);
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
  routes.value.filter((r) => (projectFilter.value === "all" || (r.project_id ?? "") === projectFilter.value) && (r.cidr + " " + r.remark + " " + projectName(r.project_id)).includes(search.value)),
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
  projects.value = await api<RouteProject[]>("/route-projects");
  if (projectFilter.value && projectFilter.value !== "all" && !projects.value.some(p => p.id === projectFilter.value)) projectFilter.value = "all";
  routes.value = await api<Route[]>("/routes");
  pending.value = await api<PendingRoutes>("/routes/pending");
}
async function save(form: RouteForm) {
  await run(async () => {
    const path = "/routes/" + selected.value?.id;
    switch (modal.value) {
      case "batch":
        await api("/routes", "POST", form);
        notice.value = "路由组已保存，应用配置后生效";
        break;
      case "edit-route":
        await api(path, "PUT", {
          project_id: form.project_id,
          lines: form.lines,
          remark: form.remark,
          enabled: selected.value?.enabled,
        });
        notice.value = "路由草稿已保存，应用配置后生效";
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
defineExpose({ refresh, isEditing });
const routeColumns = [
 { title: "项目", key: "project_id", minWidth: 130, render: (r: Route) => projectName(r.project_id) },
  { title: "目标 IP / 网段", key: "cidr", minWidth: 180,
    render: (r: Route) => r.cidr.split("\n").join(", "),
  },
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
        button("编辑", () => open("edit-route", r)),
        button("删除", () => open("delete-route", r), true),
      ]),
  },
];
</script>
<template>
  <NTabs default-value="rules" type="line" animated>
    <NTabPane name="rules" tab="规则配置" display-directive="show" :disabled="loading">
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
          <NSelect v-model:value="projectFilter" placeholder="全部项目" :options="projectOptions" style="min-width: 160px" />
          <NInput
            v-model:value="search"
            clearable
            placeholder="搜索地址、备注或项目"
          /><NButton type="primary" @click="open('batch')"
            >＋ 新增规则</NButton
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
    </NTabPane>
    <NTabPane name="projects" tab="项目配置" display-directive="show" :disabled="loading">
      <ProjectManager :projects="projects" :routes="routes" :refresh="refresh" />
    </NTabPane>
  </NTabs>
  <RouteDialog
    v-if="modal"
    :action="modal"
    :projects="projects"
    :default-project="projectFilter === 'all' ? '' : projectFilter"
    :route="selected"
    :batch="batch"
    @close="modal = ''"
    @submit="save"
  />
</template>
