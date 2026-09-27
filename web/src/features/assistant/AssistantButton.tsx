import { useEffect } from "react";
import { Bot } from "lucide-react";
import { useAssistantPanel } from "./panel";

export default function AssistantButton() {
  const toggle = useAssistantPanel((state) => state.toggle);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "j") {
        event.preventDefault();
        toggle();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggle]);
  return (
    <button
      className="icon-button"
      title="AI 助手（⌘J）"
      aria-label="打开 AI 助手"
      onClick={toggle}
    >
      <Bot size={16} />
    </button>
  );
}
