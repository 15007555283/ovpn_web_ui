import { h } from "vue";
import { NButton, NTag } from "naive-ui";
export function tag(
  text: string,
  type: "success" | "warning" | "error" | "default" = "default",
) {
  return h(NTag, { type, bordered: false, size: "small" }, () => text);
}
export function tableButton(
  text: string,
  action: () => void,
  danger = false,
  disabled = false,
) {
  return h(
    NButton,
    {
      size: "small",
      type: danger ? "error" : "default",
      quaternary: true,
      disabled,
      onClick: action,
    },
    () => text,
  );
}
