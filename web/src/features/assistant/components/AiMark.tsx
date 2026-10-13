import { useId } from "react";

/**
 * AI 的图标（B194）：一大一小两颗四角星，主题色渐变。
 * tile 时外面加一层主题色浅底的圆角方块，用在空状态。
 */
export default function AiMark({
  size = 16,
  tile = false,
  className = "",
}: {
  size?: number;
  tile?: boolean;
  className?: string;
}) {
  // useId 带冒号之类的字符，url(#…) 里不好用
  const id = `ai-mark-${useId().replace(/[^\w-]/g, "")}`;
  const svg = (
    <svg
      className={`ai-mark${tile ? "" : ` ${className}`}`}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      aria-hidden
    >
      <defs>
        <linearGradient
          id={id}
          gradientUnits="userSpaceOnUse"
          x1="3"
          y1="21"
          x2="21"
          y2="3"
        >
          <stop offset="0" className="ai-mark-from" />
          <stop offset="1" className="ai-mark-to" />
        </linearGradient>
      </defs>
      <path
        fill={`url(#${id})`}
        d="M10 4c.5 4.6 2.9 7 7.5 7.5-4.6.5-7 2.9-7.5 7.5-.5-4.6-2.9-7-7.5-7.5C7.1 11 9.5 8.6 10 4Z"
      />
      <path
        fill={`url(#${id})`}
        d="M18.5 2c.2 1.8 1.2 2.8 3 3-1.8.2-2.8 1.2-3 3-.2-1.8-1.2-2.8-3-3 1.8-.2 2.8-1.2 3-3Z"
      />
    </svg>
  );
  return tile ? (
    <span className={`ai-mark-tile ${className}`}>{svg}</span>
  ) : (
    svg
  );
}
