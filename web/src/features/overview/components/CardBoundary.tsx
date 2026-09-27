import { Component, type ReactNode } from "react";
import { AlertTriangle } from "lucide-react";
import { errorMessage } from "../../../api/client";

interface Props {
  /** 已经翻译好的卡片标题和按钮文字。类组件里用不了 useT。 */
  title: string;
  retryLabel: string;
  children: ReactNode;
}

/**
 * 包住一张卡片。卡片渲染时出错只影响这一张，其他卡片照常显示。
 * 接口报错不会走到这里，由卡片自己显示。
 */
export default class CardBoundary extends Component<Props, { error: unknown }> {
  state = { error: null as unknown };

  static getDerivedStateFromError(error: unknown) {
    return { error };
  }

  render() {
    if (this.state.error === null) return this.props.children;
    return (
      <div className="xc-card today-broken" role="alert">
        <AlertTriangle size={15} />
        <span>
          <strong>{this.props.title}</strong>
          <small>{errorMessage(this.state.error)}</small>
        </span>
        <button
          className="xc-btn small ghost"
          onClick={() => this.setState({ error: null })}
        >
          {this.props.retryLabel}
        </button>
      </div>
    );
  }
}
