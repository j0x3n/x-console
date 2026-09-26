import type * as Model from "../../types/domain";
import React, { useState, useMemo } from "react";
import IconBadge from "./IconBadge";
import { X, FileText, PenLine, Send } from "lucide-react";

interface DraftReviewDialogProps {
  language: Model.Language;
  draft: string;
  showBaseDiff?: boolean;
  setDraft: (value: string) => void;
  onClose: () => void;
  onSend: () => void;
  onSaved: () => void;
  closing: boolean;
}

export function DraftReviewDialog({
  language,
  draft,
  showBaseDiff = true,
  setDraft,
  onClose,
  onSend,
  onSaved,
  closing,
}: DraftReviewDialogProps) {
  const zh = language === "zh";
  const L = (en: string, cn?: string) => (zh ? (cn ?? en) : en);
  const [tone, setTone] = useState("As drafted");
  const [showChanges, setShowChanges] = useState(showBaseDiff);
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(draft);
  const variants = useMemo(
    () => ({
      "As drafted": draft,
      Shorter: L(
        "Hi Priya — thanks for walking us through the fleet telemetry setup yesterday.\n\nAs promised, two options sized for 40 robots now and 120 by Q2, both with decision replay and the on-prem collector.\n\nHappy to go through them at 10:00.\n\nTheo",
        "Priya，你好。感谢你昨天介绍车队遥测系统。\n\n这里有两种报价方案，分别适用于当前 40 台和第二季度 120 台机器人，都包含决策回放与本地采集器。\n\n我们可以在 10 点逐项讨论。\n\nTheo",
      ),
      Warmer: L(
        "Hi Priya — it was great speaking with you yesterday. Thanks for the thoughtful walkthrough of your fleet telemetry setup.\n\nI’ve put together two options for 40 robots now and 120 by Q2. Both include decision replay and the on-prem collector you mentioned.\n\nLooking forward to talking through them together at 10:00. Owen is welcome to join.\n\nTheo",
        "Priya，你好。昨天聊得很愉快，也感谢你详细介绍车队遥测系统。\n\n我准备了两种方案，覆盖现在的 40 台机器人和第二季度的 120 台，都包含决策回放与本地采集器。\n\n期待 10 点一起讨论，也欢迎 Owen 加入。\n\nTheo",
      ),
      "More direct": L(
        "Hi Priya — attached are two pricing options for 40 robots now and 120 by Q2. Both include decision replay and the on-prem collector.\n\nLet’s review the options at 10:00 and decide on next steps.\n\nTheo",
        "Priya，你好。附上两种报价方案，分别适用于当前 40 台和第二季度 120 台机器人。两种方案均包含决策回放和本地采集器。\n\n10 点我们可以评估方案并确定下一步。\n\nTheo",
      ),
    }),
    [draft, zh],
  );
  const selectTone = (next: keyof typeof variants) => {
    setTone(next);
    setText(variants[next]);
    setEditing(false);
  };
  const save = () => {
    setDraft(text);
    onSaved();
  };
  const send = () => {
    setDraft(text);
    onSend();
  };
  const paragraphs = text.split(/\n\s*\n/);
  return (
    <div
      className={"modal-backdrop" + (closing ? " is-closing" : "")}
      onMouseDown={onClose}
    >
      <div
        className="form-dialog draft-review-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={L("Review draft", "查看草稿")}
        onMouseDown={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
            event.preventDefault();
            send();
          }
        }}
      >
        <div className="draft-dialog-head">
          <IconBadge name="Scribe" />
          <div>
            <h2>{L("Review draft", "查看草稿")}</h2>
            <p>
              {L(
                "Scribe’s follow-up to Priya Raman · Halcyon Robotics",
                "Scribe 给 Priya Raman 的跟进邮件 · Halcyon Robotics",
              )}
            </p>
          </div>
          <button
            className="overlay-close"
            aria-label={L("Close", "关闭")}
            onClick={onClose}
          >
            <X size={16} />
          </button>
        </div>
        <dl className="draft-address">
          <div>
            <dt>{L("To", "收件人")}</dt>
            <dd>
              Priya Raman <small>&lt;priya.raman@halcyonrobotics.com&gt;</small>
            </dd>
          </div>
          <div>
            <dt>{L("From", "发件人")}</dt>
            <dd>
              Theo Park <small>&lt;theo@xcc.im&gt;</small>
            </dd>
          </div>
          <div>
            <dt>{L("Subject", "主题")}</dt>
            <dd>
              {L(
                "Two pricing options before our 10:00",
                "10 点会议前的两种报价方案",
              )}
            </dd>
          </div>
        </dl>
        <div className="draft-controls">
          <div
            className="draft-tones"
            role="radiogroup"
            aria-label={L("Tone", "语气")}
          >
            {[
              ["As drafted", "原稿"],
              ["Shorter", "简短"],
              ["Warmer", "更亲切"],
              ["More direct", "更直接"],
            ].map(([value, label]) => (
              <button
                key={value}
                role="radio"
                aria-checked={tone === value}
                className={tone === value ? "selected" : ""}
                onClick={() => selectTone(value as keyof typeof variants)}
              >
                {zh ? label : value}
              </button>
            ))}
          </div>
          {showBaseDiff && (
            <label className="draft-changes-switch">
              {L("Show changes", "显示修改")}
              <input
                type="checkbox"
                role="switch"
                checked={showChanges}
                onChange={(event) => setShowChanges(event.target.checked)}
              />
              <span />
            </label>
          )}
        </div>
        {editing ? (
          <textarea
            className="draft-text-editor"
            aria-label={L("Draft text", "邮件正文")}
            value={text}
            onChange={(event) => setText(event.target.value)}
            autoFocus
          />
        ) : (
          <div className="draft-message">
            {tone === "As drafted" && showChanges && showBaseDiff && zh ? (
              <>
                <p>
                  Priya，你好。感谢你昨天<del>抽出时间</del>
                  <ins>介绍车队遥测系统</ins>。
                  <ins>
                    Jun 提到的拣货机器人之间如何追踪任务交接，正是 X Console
                    能发挥作用的地方。
                  </ins>
                </p>
                <p>
                  按约定，<del>这里是我们的报价方案。</del>
                  <ins>
                    我们准备了两种方案，分别适用于目前的 40
                    台机器人和第二季度扩展至 120
                    台的需求。两种方案都包含决策回放及你提出的本地采集器。
                  </ins>
                </p>
                <p>
                  我们可以<del>在通话中讨论。</del>
                  <ins>在 10 点逐项讨论，也欢迎 Owen 加入。</ins>
                </p>
                <p>Theo</p>
              </>
            ) : tone === "As drafted" && showChanges && showBaseDiff ? (
              <>
                <p>
                  Hi Priya — thanks for <del>your time</del>
                  <ins>walking us through the fleet telemetry setup</ins>{" "}
                  yesterday.{" "}
                  <ins>
                    Jun’s question about tracing handoffs between picking agents
                    is exactly where X Console earns its keep.
                  </ins>
                </p>
                <p>
                  As promised, <del>here are our pricing options.</del>
                  <ins>
                    two options sized for 40 robots now and 120 by Q2. Both
                    include decision replay and the on-prem collector you asked
                    about.
                  </ins>
                </p>
                <p>
                  Happy to <del>walk through them on our call.</del>
                  <ins>
                    go through them line by line at 10:00. Owen is welcome to
                    join.
                  </ins>
                </p>
                <p>Theo</p>
              </>
            ) : (
              paragraphs.map((paragraph, index) => (
                <p key={index}>{paragraph}</p>
              ))
            )}
          </div>
        )}
        <div className="draft-receipts">
          <span>
            <FileText size={13} />{" "}
            {L(
              "Demo call, Sep 23 · 47 min transcript · Pricing sheet, v4",
              "演示通话，9 月 23 日 · 47 分钟记录 · 报价表 v4",
            )}
          </span>
          {showChanges && showBaseDiff && !editing && (
            <small>
              <i />
              {L("added", "新增")} <i />
              {L(
                "removed · vs Theo’s usual follow-up",
                "删除 · 对比 Theo 的常用跟进邮件",
              )}
            </small>
          )}
        </div>
        <div className="draft-dialog-footer">
          <span>
            {L(
              "Sends as Theo. Logged to Halcyon Robotics.",
              "将以 Theo 的名义发送，并记录至 Halcyon Robotics。",
            )}
          </span>
          <button
            className="draft-edit"
            onClick={() => setEditing((value) => !value)}
          >
            <PenLine size={14} />{" "}
            {editing ? L("Preview", "预览") : L("Edit text", "编辑正文")}
          </button>
          <button className="draft-save" onClick={save}>
            {L("Save draft", "保存草稿")}
          </button>
          <button className="draft-send" onClick={send}>
            <Send size={14} />
            {L("Send follow-up", "发送跟进邮件")}
            <kbd>⌘↵</kbd>
          </button>
        </div>
      </div>
    </div>
  );
}
