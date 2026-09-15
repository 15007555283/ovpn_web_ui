<script setup lang="ts">
import { reactive } from "vue";
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NTag,
} from "naive-ui";
import { useAppContext } from "../../composables/useAppContext";
import type { Route, BatchResult } from "../../types";
import type { RouteAction, RouteForm } from "./types";
const props = defineProps<{
  action: RouteAction;
  route?: Route;
  batch: BatchResult[];
}>();
const emit = defineEmits<{ close: []; submit: [form: RouteForm] }>();
const { loading, error } = useAppContext();
const form = reactive<RouteForm>({
  lines: "",
  remark: props.route?.remark ?? "",
});
const titles: Record<RouteAction, string> = {
  batch: "新增分流路由",
  "edit-route": "编辑路由备注",
  "delete-route": "删除分流路由",
  apply: "应用路由配置",
};
</script>
<template>
  <NModal
    :show="true"
    preset="card"
    :title="titles[action]"
    :style="{ width: 'min(580px, calc(100vw - 32px))' }"
    :mask-closable="!loading"
    :closable="!loading"
    @update:show="
      (value) => {
        if (!value) emit('close');
      }
    "
  >
    <NAlert v-if="error" type="error" class="banner">{{ error }}</NAlert>
    <NForm v-if="action === 'batch' || action === 'edit-route'">
      <NFormItem
        v-if="action === 'batch'"
        label="目标 IP / CIDR（每行一条，最多 500 行）"
        ><NInput
          v-model:value="form.lines"
          type="textarea"
          :rows="6"
          placeholder="8.8.8.8&#10;20.30.40.0/24"
      /></NFormItem>
      <NFormItem label="备注"
        ><NInput v-model:value="form.remark" type="textarea" :rows="3"
      /></NFormItem>
      <div v-for="result in batch" :key="result.line" class="batch-result">
        <NTag :type="result.success ? 'success' : 'error'" size="small"
          >第 {{ result.line }} 行</NTag
        >
        {{ result.input }} · {{ result.message }}
      </div>
    </NForm>
    <template v-else>
      <NAlert type="warning" :bordered="false">{{
        action === "apply"
          ? "应用配置会用已启用路由替换服务器现有分流规则，并重启 OpenVPN、中断当前连接。客户端需要重新连接才能收到新路由。"
          : "删除规则后需应用配置才会生效。"
      }}</NAlert>
      <p v-if="action === 'delete-route'">{{ route?.cidr }}</p>
    </template>
    <template #footer
      ><div class="modal-footer">
        <NButton :disabled="loading" @click="emit('close')">取消</NButton>
        <NButton
          :type="
            action === 'apply' || action === 'delete-route'
              ? 'error'
              : 'primary'
          "
          :loading="loading"
          @click="emit('submit', form)"
          >{{ action === "batch" ? "添加路由" : "确认" }}</NButton
        >
      </div></template
    >
  </NModal>
</template>
