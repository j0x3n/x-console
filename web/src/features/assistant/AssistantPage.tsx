import PageHeading from "../../components/ui/PageHeading";
import ChatView from "./ChatView";

export default function AssistantPage() {
  return (
    <div className="xc-page wide assistant-page">
      <PageHeading title="AI 助手" />
      <ChatView />
    </div>
  );
}
