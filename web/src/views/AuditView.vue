<script setup lang="ts">
import { ref } from "vue";
import { NButton, NInput, NPagination } from "naive-ui";
import AuditTable from "../components/AuditTable.vue";
import { api } from "../api/client";
import { useAppContext } from "../composables/useAppContext";
import type { AuditLog } from "../types";
const { loading, run } = useAppContext();
const logs = ref<AuditLog[]>([]),
  total = ref(0),
  auditPage = ref(1),
  search = ref("");
async function refresh() {
  const data = await api<{ items: AuditLog[]; total: number }>(
    `/audit-logs?page=${auditPage.value}&q=${encodeURIComponent(search.value)}`,
  );
  logs.value = data.items;
  total.value = data.total;
}
function searchLogs() {
  return run(async () => {
    auditPage.value = 1;
    await refresh();
  });
}
defineExpose({ refresh });
</script>
<template>
  <section class="panel">
    <div class="toolbar">
      <NInput
        v-model:value="search"
        placeholder="搜索动作、资源或请求 ID"
        @keyup.enter="searchLogs"
      /><NButton @click="searchLogs">搜索</NButton>
    </div>
    <AuditTable :data="logs" :loading="loading" /><NPagination
      v-model:page="auditPage"
      :item-count="total"
      :page-size="50"
      class="pagination"
      @update:page="run(refresh)"
    />
  </section>
</template>
