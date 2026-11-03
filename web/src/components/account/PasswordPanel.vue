<script setup lang="ts">
import { reactive } from "vue";
import { NButton, NForm, NFormItem, NInput } from "naive-ui";
import { api } from "../../api/client";
import { useAppContext } from "../../composables/useAppContext";
const { loading, logged, notice, run } = useAppContext();
const password = reactive({ old_password: "", new_password: "" });
async function changePassword() {
  await run(async () => {
    await api("/auth/password", "PUT", password);
    password.old_password = "";
    password.new_password = "";
    logged.value = false;
    notice.value = "密码已修改，请重新登录";
  });
}
</script>
<template>
  <section>
    <NForm
      ><NFormItem label="原密码"
        ><NInput
          v-model:value="password.old_password"
          type="password"
          autocomplete="current-password" /></NFormItem
      ><NFormItem label="新密码（12～72 字节）"
        ><NInput
          v-model:value="password.new_password"
          type="password"
          autocomplete="new-password" /></NFormItem
      ><NButton :loading="loading" @click="changePassword"
        >修改密码并退出</NButton
      ></NForm
    >
  </section>
</template>
