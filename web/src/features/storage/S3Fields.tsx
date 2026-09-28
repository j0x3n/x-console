import { useT } from "../../contexts/LanguageContext";
import type { StorageS3Input } from "./api";
import "./i18n";

export type S3Form = {
  endpoint: string;
  region: string;
  bucket: string;
  prefix: string;
  accessKeyId: string;
  secretAccessKey: string;
  pathStyle: boolean;
};

export function emptyS3(s3?: Partial<S3Form>, prefix = "x-console"): S3Form {
  return {
    endpoint: s3?.endpoint ?? "",
    region: s3?.region ?? "",
    bucket: s3?.bucket ?? "",
    prefix: s3?.prefix ?? prefix,
    accessKeyId: s3?.accessKeyId ?? "",
    secretAccessKey: "",
    pathStyle: s3?.pathStyle ?? false,
  };
}

/** 只把要保存的字段发出去。Secret 留空表示不改。 */
export function s3Input(form: S3Form): StorageS3Input {
  const out: StorageS3Input = {
    endpoint: form.endpoint.trim(),
    region: form.region.trim(),
    bucket: form.bucket.trim(),
    prefix: form.prefix.trim().replace(/^\/+|\/+$/g, ""),
    accessKeyId: form.accessKeyId.trim(),
    pathStyle: form.pathStyle,
  };
  if (form.secretAccessKey.trim())
    out.secretAccessKey = form.secretAccessKey.trim();
  return out;
}

/** S3 的几项输入。存储设置和备份设置共用。 */
export default function S3Fields({
  value,
  onChange,
  hasSecret,
}: {
  value: S3Form;
  onChange: (next: S3Form) => void;
  hasSecret?: boolean;
}) {
  const t = useT();
  const set = <K extends keyof S3Form>(key: K, v: S3Form[K]) =>
    onChange({ ...value, [key]: v });
  const field = (
    key: "endpoint" | "region" | "bucket" | "prefix" | "accessKeyId",
    label: string,
    placeholder: string,
    hint?: string,
  ) => (
    <label className="xc-field">
      <span>{label}</span>
      <input
        className="xc-input"
        value={value[key]}
        onChange={(e) => set(key, e.target.value)}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
      />
      {hint && <small>{hint}</small>}
    </label>
  );
  return (
    <>
      {field("endpoint", t("Endpoint"), "https://s3.amazonaws.com")}
      {field("region", t("Region"), "us-east-1", "R2 填 auto。")}
      {field("bucket", t("Bucket"), "my-console")}
      {field(
        "prefix",
        t("Path prefix"),
        "x-console",
        "文件放在桶里的这个目录下。",
      )}
      {field("accessKeyId", "Access Key ID", "")}
      <label className="xc-field">
        <span>Secret Access Key</span>
        <input
          className="xc-input"
          type="password"
          value={value.secretAccessKey}
          onChange={(e) => set("secretAccessKey", e.target.value)}
          placeholder={hasSecret ? t("leave empty to keep") : ""}
          autoComplete="new-password"
        />
      </label>
      <label className="xc-check">
        <input
          type="checkbox"
          checked={value.pathStyle}
          onChange={(e) => set("pathStyle", e.target.checked)}
        />
        <span>{t("Path-style URLs")}</span>
      </label>
      <small className="xc-check-hint">MinIO 一般要勾上。</small>
    </>
  );
}
