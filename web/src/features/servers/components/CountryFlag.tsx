import { flagEmoji } from "../api";

/**
 * 国旗（B82）。用 emoji，Windows 上靠 servers.css 里的国旗字体显示。
 * 鼠标移上去提示国家名。
 */
export default function CountryFlag({
  country,
}: {
  country: { code: string; name: string };
}) {
  const flag = flagEmoji(country.code);
  if (!flag) return null;
  return (
    <span
      className="xc-flag"
      title={country.name}
      role="img"
      aria-label={country.name}
    >
      {flag}
    </span>
  );
}
