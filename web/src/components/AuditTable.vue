<script setup lang="ts">
import { h } from "vue";
import { NDataTable } from "naive-ui";
import { fmt } from "../utils/format";
import { tag } from "../utils/table";
import type { AuditLog } from "../types";
defineProps<{ data: AuditLog[]; loading: boolean }>();
const auditColumns = [
  {
    title: "时间",
    key: "created_at",
    minWidth: 170,
    render: (r: AuditLog) => fmt(r.created_at),
  },
  { title: "管理员", key: "admin", minWidth: 100 },
  { title: "操作", key: "action", minWidth: 130 },
  { title: "资源", key: "resource_id", minWidth: 130 },
  { title: "来源 IP", key: "source_ip", minWidth: 100 },
  {
    title: "结果",
    key: "success",
    minWidth: 85,
    render: (r: AuditLog) =>
      tag(r.success ? "成功" : "失败", r.success ? "success" : "error"),
  },
  {
    title: "摘要 / 请求 ID",
    key: "detail",
    minWidth: 230,
    render: (r: AuditLog) =>
      h("div", [h("div", r.detail), h("small", r.request_id)]),
  },
];
</script>
<template>
  <NDataTable
    :columns="auditColumns"
    :data="data"
    :loading="loading"
    :scroll-x="1050"
  />
</template>
