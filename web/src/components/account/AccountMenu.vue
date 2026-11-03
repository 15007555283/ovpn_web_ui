<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import { NAlert, NButton, NDropdown, NModal } from "naive-ui";
import { api } from "../../api/client";
import { useAppContext } from "../../composables/useAppContext";
import type { PageView } from "../../types";
import SystemPanel from "./SystemPanel.vue";
import SettingsPanel from "./SettingsPanel.vue";
import PasswordPanel from "./PasswordPanel.vue";

const { loading, error, notice, logged, settings, run } = useAppContext();
type DialogKey = "system" | "settings" | "password";
const active = ref<DialogKey>();
const content = ref<PageView>();
const panels = { system: SystemPanel, settings: SettingsPanel, password: PasswordPanel };
const titles = { system: "系统状态", settings: "系统设置", password: "修改密码" };
const options = computed(() => [
  ...Object.entries(titles).map(([key, label]) => ({ key, label, disabled: loading.value })),
  { type: "divider", key: "divider" },
  { key: "logout", label: "退出登录", disabled: loading.value },
]);
const isOpen = computed(() => !!active.value);
function close() {
  if (!loading.value) active.value = undefined;
}
async function select(key: string) {
  await run(async () => {
    if (key === "logout") {
      await api("/auth/logout", "POST");
      logged.value = false;
      return;
    }
    if (key !== "system" && key !== "settings" && key !== "password") return;
    active.value = key;
    await nextTick();
    await content.value?.refresh?.();
    lastRefresh = Date.now();
  });
}
let timer: ReturnType<typeof setInterval> | undefined;
let lastRefresh = Date.now();
onMounted(() => {
  timer = setInterval(() => {
    if (active.value === "system" && !loading.value && !content.value?.isEditing && Date.now() - lastRefresh >= settings.refresh * 1000) {
      lastRefresh = Date.now();
      run(async () => { await content.value?.refresh(); });
    }
  }, 1000);
});
onUnmounted(() => clearInterval(timer));
defineExpose({ isOpen });
</script>
<template>
  <NDropdown trigger="click" placement="bottom-end" :options="options" @select="select">
    <NButton secondary circle type="primary" class="account-trigger" :disabled="loading" aria-label="管理员菜单">
      <template #icon>
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <circle cx="12" cy="8" r="4" />
          <path d="M4 21v-2a8 8 0 0 1 16 0v2" />
        </svg>
      </template>
    </NButton>
  </NDropdown>
  <NModal
    :show="isOpen"
    preset="card"
    :title="active ? titles[active] : ''"
    :style="{ width: `min(${active === 'password' ? 480 : 860}px, calc(100vw - 32px))` }"
    :mask-closable="!loading"
    :closable="!loading"
    :close-on-esc="!loading"
    @update:show="value => { if (!value) close(); }"
  >
    <NAlert v-if="error" type="error" class="banner">{{ error }}</NAlert>
    <NAlert v-if="notice" type="success" class="banner">{{ notice }}</NAlert>
    <component v-if="active" :is="panels[active]" :key="active" ref="content" />
  </NModal>
</template>
<style scoped>
.account-trigger {
  width: 40px;
  height: 40px;
  min-width: 40px;
  padding: 0;
  border-radius: 50%;
  flex-shrink: 0;
}
</style>
