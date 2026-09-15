export const fmt = (value?: string) =>
  !value || value.startsWith("0001")
    ? "—"
    : new Date(value).toLocaleString("zh-CN");
export function bytes(n: number) {
  if (!n) return "0 B";
  const i = Math.min(3, Math.floor(Math.log(n) / Math.log(1024)));
  return (n / 1024 ** i).toFixed(1) + " " + ["B", "KB", "MB", "GB"][i];
}
