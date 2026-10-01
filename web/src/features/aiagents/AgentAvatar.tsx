import type { AiAgent } from "./api";

/** Agent 的头像：彩色圆底上一个 emoji，没有时用名字的第一个字。只是装饰，名字由旁边的文字或外层的 aria-label 给出。 */
export default function AgentAvatar({
  agent,
  size = 24,
  failed,
}: {
  agent: Pick<AiAgent, "name" | "avatar" | "color"> | undefined;
  size?: number;
  failed?: boolean;
}) {
  const name = agent?.name ?? "?";
  return (
    <span
      className={`aiagent-avatar${failed ? " failed" : ""}`}
      style={{
        width: size,
        height: size,
        fontSize: Math.round(size * 0.55),
        background: agent?.color || undefined,
      }}
      title={name}
      aria-hidden="true"
    >
      {agent?.avatar || Array.from(name)[0]}
    </span>
  );
}
