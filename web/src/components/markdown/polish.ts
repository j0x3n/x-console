import { apiFetch } from "../../api/client";
import type { components } from "../../api/gen/ai";

/** 润色的场景（B56），后端按它用不同的提示词。 */
export type PolishScene = components["schemas"]["PolishScene"];

/** 调 POST /ai/polish，返回润色后的 Markdown。 */
export async function polishText(
  scene: PolishScene,
  text: string,
  prompt?: string,
): Promise<string> {
  const res = await apiFetch("/ai/polish", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text, scene, ...(prompt ? { prompt } : {}) }),
  });
  const out = (await res.json()) as components["schemas"]["PolishResult"];
  return out.text;
}
