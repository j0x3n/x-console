/** 开关。列表里“开启 / 关闭”一条规则、一个功能时用。表单里的是非选项用 .xc-check 勾选框。 */
export default function Switch({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  /** 读屏文字，比如“CPU 太高时提醒我 开关”。 */
  label: string;
  disabled?: boolean;
}) {
  return (
    <label className="xc-switch" title={label}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        aria-label={label}
        onChange={(e) => onChange(e.target.checked)}
      />
      <i />
    </label>
  );
}
