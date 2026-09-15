import { inject, provide, reactive, ref, type InjectionKey } from "vue";
import { ApiError } from "../api/client";
import type { Settings } from "../types";

function createContext() {
  const loading = ref(false),
    error = ref(""),
    notice = ref("");
  const logged = ref(false),
    fake = ref(false);
  const settings = reactive<Settings>({
    endpoint: "vpn.example.com",
    port: 1194,
    protocol: "udp",
    dns: "",
    refresh: 10,
    session_hours: 8,
    name: "VPN Admin",
  });
  async function run(action: () => Promise<void>) {
    if (loading.value) return;
    loading.value = true;
    error.value = "";
    notice.value = "";
    try {
      await action();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) logged.value = false;
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }
  return { loading, error, notice, logged, fake, settings, run };
}
const contextKey: InjectionKey<ReturnType<typeof createContext>> =
  Symbol("app");
export function provideAppContext() {
  const context = createContext();
  provide(contextKey, context);
  return context;
}
export function useAppContext() {
  const context = inject(contextKey);
  if (!context) throw new Error("应用上下文尚未初始化");
  return context;
}
