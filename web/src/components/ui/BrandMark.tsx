/** D 方案站标。页面内跟随主题色，浏览器和安装图标使用默认靛蓝。 */
export default function BrandMark({ size = 32 }: { size?: number | string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 64 64"
      width={size}
      height={size}
      aria-hidden="true"
      focusable="false"
      style={{ verticalAlign: "middle", flexShrink: 0 }}
    >
      <rect width="64" height="64" rx="16" fill="var(--xc-accent)" />
      <g fill="var(--xc-on-accent)" stroke="var(--xc-on-accent)">
        <path
          d="M19 19 27 27M37 37 45 45M45 19 37 27M27 37 19 45"
          fill="none"
          strokeWidth="4.5"
        />
        <circle cx="19" cy="19" r="6" stroke="none" />
        <circle cx="45" cy="19" r="6" stroke="none" />
        <circle cx="19" cy="45" r="6" stroke="none" />
        <circle cx="45" cy="45" r="6" stroke="none" />
        <circle cx="32" cy="32" r="5" fill="none" strokeWidth="4" />
      </g>
    </svg>
  );
}
