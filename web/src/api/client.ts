export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
async function check(response: Response) {
  if (response.ok) return;
  const result = await response.json();
  throw new ApiError(
    response.status,
    `${result.message}（请求 ID：${result.request_id}）`,
  );
}
export async function api<T = void>(
  path: string,
  method = "GET",
  data?: unknown,
): Promise<T> {
  const response = await fetch("/api/v1" + path, {
    method,
    headers: { "Content-Type": "application/json", "X-VPN-Admin": "1" },
    body: data === undefined ? undefined : JSON.stringify(data),
  });
  await check(response);
  return (await response.json()).data as T;
}
export async function downloadFile(path: string, filename: string) {
  const response = await fetch("/api/v1" + path);
  await check(response);
  const url = URL.createObjectURL(await response.blob());
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
