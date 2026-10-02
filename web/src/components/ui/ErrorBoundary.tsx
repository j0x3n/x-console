import { Component, type ReactNode } from "react";
import { reportError } from "../../lib/errors";
import { isChunkLoadError, reloadOnce } from "../../lib/chunkReload";
import { translate } from "../../lib/i18n";
import { usePreferencesStore } from "../../stores/preferences-store";

/** 页面渲染出错时显示说明和重新加载按钮，并上报（B41）。 */
export default class ErrorBoundary extends Component<
  { children: ReactNode },
  { error: Error | null }
> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error) {
    // 部署后旧页面取不到按需加载的文件：刷新一次拿新版本，不报错
    if (isChunkLoadError(error) && reloadOnce()) return;
    reportError(error);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    const t = (text: string) =>
      translate(usePreferencesStore.getState().language, text);
    return (
      <div className="error-boundary" role="alert">
        <h2>{t("Something went wrong on this page")}</h2>
        <pre>{error.message}</pre>
        <button className="xc-btn" onClick={() => location.reload()}>
          {t("Reload")}
        </button>
      </div>
    );
  }
}
