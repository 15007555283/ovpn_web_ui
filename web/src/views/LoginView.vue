<script setup lang="ts">
import { reactive } from "vue";
import { NAlert, NButton, NForm, NFormItem, NInput } from "naive-ui";
import { api } from "../api/client";
import { useAppContext } from "../composables/useAppContext";
const setup = defineModel<boolean>("setup", { required: true });
const emit = defineEmits<{ authenticated: [] }>();
const { fake, error, notice, loading, run } = useAppContext();
const auth = reactive({ username: "", password: "" });
async function submitAuth() {
  let authenticated = false;
  await run(async () => {
    if (setup.value) {
      await api("/setup/admin", "POST", auth);
      setup.value = false;
      notice.value = "管理员已初始化，请登录";
    } else {
      await api("/auth/login", "POST", auth);
      authenticated = true;
    }
    auth.password = "";
  });
  if (authenticated) emit("authenticated");
}
</script>
<template>
  <div class="login-screen">
    <div class="login-intro">
      <span class="brand-mark">V</span>
      <div class="eyebrow">PRIVATE NETWORK · SIMPLE CONTROL</div>
      <h1>你的网络，<br />尽在掌握。</h1>
      <p>统一管理 VPN 用户、分流路由和连接状态。</p>
      <div class="intro-foot">OpenVPN Community · 单节点管理控制台</div>
    </div>
    <section class="login-card">
      <span class="eyebrow">VPN ADMIN</span>
      <h2>{{ setup ? "初始化管理员" : "欢迎回来" }}</h2>
      <p class="muted">
        {{
          setup
            ? "创建唯一管理员账号，初始化后此入口将关闭。"
            : "登录以管理你的专属 VPN 网络。"
        }}
      </p>
      <NAlert v-if="fake" type="warning" :bordered="false"
        >演示模式，不会操作真实 VPN</NAlert
      ><NAlert v-if="error" type="error">{{ error }}</NAlert
      ><NAlert v-if="notice" type="success">{{ notice }}</NAlert
      ><NForm @submit.prevent="submitAuth"
        ><NFormItem label="管理员账号"
          ><NInput
            v-model:value="auth.username"
            placeholder="输入管理员账号"
            autocomplete="username" /></NFormItem
        ><NFormItem label="密码"
          ><NInput
            v-model:value="auth.password"
            type="password"
            show-password-on="click"
            :placeholder="setup ? '至少 12 个字符' : '输入密码'"
            :autocomplete="
              setup ? 'new-password' : 'current-password'
            " /></NFormItem
        ><NButton
          attr-type="submit"
          type="primary"
          block
          size="large"
          :loading="loading"
          >{{ setup ? "创建管理员" : "登录控制台" }}</NButton
        ></NForm
      >
    </section>
  </div>
</template>
