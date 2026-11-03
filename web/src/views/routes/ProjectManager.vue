<script setup lang="ts">
import { h, reactive, shallowRef } from "vue";
import { NAlert, NModal, NButton, NDataTable, NForm, NFormItem, NInput, NPopconfirm } from "naive-ui";
import { api } from "../../api/client";
import { useAppContext } from "../../composables/useAppContext";
import type { Route, RouteProject } from "../../types";
const props = defineProps<{ projects: RouteProject[]; routes: Route[]; refresh: () => Promise<void> }>();
const { loading, error, run } = useAppContext();
const editing = shallowRef("");
const showForm = shallowRef(false);
const form = reactive({ name: "", remark: "" });
function edit(project?: RouteProject) {
  error.value = "";
  editing.value = project?.id ?? "";
  form.name = project?.name ?? "";
  form.remark = project?.remark ?? "";
  showForm.value = true;
}
function save() {
  if (loading.value) return;
  return run(async () => {
    await api("/route-projects" + (editing.value ? "/" + editing.value : ""), editing.value ? "PUT" : "POST", form);
    showForm.value = false;
    await props.refresh();
  });
}
function remove(project: RouteProject) {
  return run(async () => {
    await api("/route-projects/" + project.id, "DELETE");
    await props.refresh();
  });
}
const columns = [
  { title: "项目", key: "name" },
  { title: "备注", key: "remark" },
  { title: "规则数", key: "count", render: (p: RouteProject) => props.routes.filter(r => r.project_id === p.id).length },
  { title: "操作", key: "actions", render: (p: RouteProject) => h("div", {class: "actions"}, [
    h(NButton, {size: "small", disabled: loading.value, onClick: () => edit(p)}, {default: () => "编辑"}),
    h(NPopconfirm, {onPositiveClick: () => remove(p)}, {
      trigger: () => h(NButton, {size: "small", type: "error", secondary: true, disabled: loading.value || props.routes.some(r => r.project_id === p.id)}, {default: () => "删除"}),
      default: () => "确认删除这个空项目？",
    }),
  ]) },
];
</script>
<template>
  <section class="panel">
    <p class="muted">项目仅用于整理规则，不隔离 VPN 用户。删除项目之前，请先移出其中的规则；已有规则默认属于“未分组”。</p>
    <div class="toolbar">
      <NButton type="primary" :disabled="loading" @click="edit()">＋ 新增项目</NButton>
    </div>
    <NDataTable :columns="columns" :data="projects" :loading="loading" :pagination="{pageSize: 5}" />
  </section>
  <NModal
    v-model:show="showForm"
    preset="card"
    :title="editing ? '编辑项目' : '新增项目'"
    :style="{ width: 'min(560px, calc(100vw - 32px))' }"
    :mask-closable="!loading"
    :closable="!loading"
    :close-on-esc="!loading"
  >
    <NAlert v-if="error" type="error" class="banner">{{ error }}</NAlert>
    <NForm @submit.prevent="save">
      <NFormItem label="项目名称"><NInput v-model:value="form.name" :maxlength="80" placeholder="例如：公司业务、个人服务" /></NFormItem>
      <NFormItem label="备注"><NInput v-model:value="form.remark" type="textarea" :rows="2" /></NFormItem>
      <div class="actions">
        <NButton type="primary" :loading="loading" attr-type="submit">{{ editing ? '保存项目' : '新增项目' }}</NButton>
        <NButton :disabled="loading" @click="showForm = false">取消</NButton>
      </div>
    </NForm>
  </NModal>
</template>
