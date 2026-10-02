import { useEffect, useState, type FormEvent } from "react";
import { useAuthStatus } from "../../api/core";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { usePreferencesStore } from "../../stores/preferences-store";

/** B88：今日页问候语里的称呼。存在个人偏好里，换浏览器也一样。 */
export default function NicknameCard() {
  const t = useT();
  const auth = useAuthStatus();
  const nickname = usePreferencesStore((s) => s.nickname);
  const setNickname = usePreferencesStore((s) => s.setNickname);
  const [value, setValue] = useState(nickname);
  useEffect(() => setValue(nickname), [nickname]);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    setNickname(value);
    toast(t("Saved"));
  };
  return (
    <form className="xc-card" onSubmit={submit}>
      <div className="xc-card-head">
        <h2>{t("What to call you")}</h2>
      </div>
      <div className="xc-field">
        <label htmlFor="settings-nickname">{t("Name in the greeting")}</label>
        <input
          id="settings-nickname"
          className="xc-input"
          maxLength={20}
          value={value}
          placeholder={auth.data?.username}
          onChange={(e) => setValue(e.target.value)}
        />
        <small>
          {t(
            "Shown on the today page, like “Good morning, …”. Empty uses your login name.",
          )}
        </small>
      </div>
      <div className="xc-row">
        <span className="xc-spacer" />
        <button
          className="xc-btn small primary"
          disabled={value.trim() === nickname}
        >
          {t("Save")}
        </button>
      </div>
    </form>
  );
}
