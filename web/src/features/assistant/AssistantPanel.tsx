import { X } from "lucide-react";
import { Link } from "react-router";
import ChatView from "./ChatView";
import { useAssistantPanel } from "./panel";

export default function AssistantPanel() {
  const open = useAssistantPanel((state) => state.open);
  const close = useAssistantPanel((state) => state.close);
  if (!open) return null;
  return (
    <>
      <button
        className="assistant-scrim"
        aria-label="关闭 AI 助手"
        onClick={close}
      />
      <aside className="assistant-panel" aria-label="AI 助手">
        <header>
          <strong>AI 助手</strong>
          <Link to="/assistant" onClick={close}>
            打开完整页面
          </Link>
          <button className="icon-button" aria-label="关闭" onClick={close}>
            <X size={16} />
          </button>
        </header>
        <ChatView compact />
      </aside>
    </>
  );
}
