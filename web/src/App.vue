<script setup lang="ts">
import { nextTick, onMounted, ref } from "vue";
import { NConfigProvider, zhCN, dateZhCN } from "naive-ui";
import { api, ApiError } from "./api/client";
import { provideAppContext } from "./composables/useAppContext";
import AdminLayout from "./layouts/AdminLayout.vue";
import LoginView from "./views/LoginView.vue";
import type { PageView, Settings } from "./types";

const { settings, logged, fake, run } = provideAppContext();
const setup = ref(false);
const layout = ref<PageView>();
async function enter() {
  Object.assign(settings, await api<Settings>("/settings"));
  logged.value = true;
  await nextTick();
  await layout.value?.refresh();
}
onMounted(() =>
  run(async () => {
    const state = await api<{ needs_setup: boolean; fake: boolean }>(
      "/setup/status",
    );
    setup.value = state.needs_setup;
    fake.value = state.fake;
    if (setup.value) return;
    try {
      await api("/auth/me");
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return;
      throw e;
    }
    await enter();
  }),
);
</script>

<template>
  <NConfigProvider
    :locale="zhCN"
    :date-locale="dateZhCN"
    :theme-overrides="{
      common: {
        primaryColor: '#0d776f',
        primaryColorHover: '#12958a',
        primaryColorPressed: '#09665f',
        borderRadius: '8px',
      },
    }"
  >
    <LoginView
      v-if="!logged"
      v-model:setup="setup"
      @authenticated="run(enter)"
    />
    <AdminLayout v-else ref="layout" />
  </NConfigProvider>
</template>
