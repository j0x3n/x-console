import { ErrorState, Loading } from "../../components/ui/States";
import { useAiProviders } from "./api";
import "./i18n";
import "./assistant.css";
import AiSettingsView from "./settings/AiSettingsView";

export default function AssistantSettingsTab() {
  const providers = useAiProviders();
  if (providers.isPending) return <Loading />;
  if (providers.isError)
    return (
      <ErrorState error={providers.error} onRetry={() => providers.refetch()} />
    );
  return <AiSettingsView providers={providers.data} />;
}
