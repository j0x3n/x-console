import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateDocument,
  useCreateKind,
  useCustomKinds,
  useDeleteKind,
  useUpdateDocument,
  type DocumentItem,
  type DocumentKind,
} from "./api";
import { KINDS, KIND_LABELS, formatDays, parseDays } from "./format";

const NEW_KIND = "__new__";

const CURRENCIES = ["CNY", "USD", "EUR", "HKD", "JPY", "GBP", "SGD"];

interface Props {
  open: boolean;
  onClose: () => void;
  /** 传了就是修改 */
  document?: DocumentItem | null;
  /** 新建时默认的类型 */
  defaultKind?: DocumentKind;
}

/** 新建或修改一份档案。 */
export default function DocumentDialog({
  open,
  onClose,
  document: doc,
  defaultKind,
}: Props) {
  const t = useT();
  const create = useCreateDocument();
  const update = useUpdateDocument();
  const customKinds = useCustomKinds().data ?? [];
  const createKind = useCreateKind();
  const deleteKind = useDeleteKind();
  const [newKind, setNewKind] = useState<string | null>(null);
  const [kind, setKind] = useState<DocumentKind>("passport");
  const [name, setName] = useState("");
  const [holder, setHolder] = useState("");
  const [number, setNumber] = useState("");
  const [issuedOn, setIssuedOn] = useState("");
  const [expiresOn, setExpiresOn] = useState("");
  const [price, setPrice] = useState("");
  const [currency, setCurrency] = useState("CNY");
  const [serial, setSerial] = useState("");
  const [remind, setRemind] = useState("90, 30, 7");
  const [notes, setNotes] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setError("");
    setKind(doc?.kind ?? defaultKind ?? "passport");
    setNewKind(null);
    setName(doc?.name ?? "");
    setHolder(doc?.holder ?? "");
    setNumber(doc?.number ?? "");
    setIssuedOn(doc?.issuedOn ?? "");
    setExpiresOn(doc?.expiresOn ?? "");
    setPrice(doc?.price != null ? String(doc.price) : "");
    setCurrency(doc?.currency || "CNY");
    setSerial(doc?.serial ?? "");
    setRemind(doc ? formatDays(doc.remindDays) : "90, 30, 7");
    setNotes(doc?.notes ?? "");
  }, [open, doc, defaultKind]);

  const isItem = kind === "item" || !!doc?.price || !!doc?.serial;
  const saving = create.isPending || update.isPending;

  // 只有没有档案在用的自定义类型能删
  const removableKind = customKinds.find(
    (k) => k.key === kind && k.count === 0,
  );
  const addKind = async () => {
    const typed = (newKind ?? "").trim();
    if (!typed) return;
    try {
      const created = await createKind.mutateAsync(typed);
      setKind(created.key);
      setNewKind(null);
    } catch (err) {
      setError(errorMessage(err));
    }
  };
  const removeKind = async () => {
    if (!removableKind) return;
    try {
      await deleteKind.mutateAsync(removableKind.name);
      setKind("passport");
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (newKind !== null) return setError(t("Add the new type first"));
    if (!name.trim()) return setError(t("Please enter a name"));
    const days = parseDays(remind);
    if (!days)
      return setError(
        t("Remind days: whole numbers from 1 to 3650, at most 8"),
      );
    const value = price.trim() === "" ? null : Number(price);
    if (value !== null && (!Number.isFinite(value) || value < 0))
      return setError(t("Price"));
    const fields = {
      kind,
      name: name.trim(),
      holder: holder.trim(),
      number: number.trim(),
      issuedOn,
      expiresOn,
      serial: serial.trim(),
      currency: value === null ? "" : currency,
      remindDays: days,
      notes,
    };
    try {
      if (doc)
        await update.mutateAsync({
          id: doc.id,
          body: { ...fields, ...(value === null ? {} : { price: value }) },
        });
      else
        await create.mutateAsync({
          ...fields,
          ...(value === null ? {} : { price: value }),
        });
      toast(t("Saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={doc ? t("Edit document") : t("New document")}
      wide
    >
      <form onSubmit={submit}>
        <div className="documents-form-row">
          <div className="documents-narrow documents-kind">
            <label className="xc-field">
              <span>{t("Type")}</span>
              <select
                className="xc-select"
                value={newKind === null ? kind : NEW_KIND}
                onChange={(e) => {
                  if (e.target.value === NEW_KIND) return setNewKind("");
                  setNewKind(null);
                  setKind(e.target.value as DocumentKind);
                }}
              >
                {KINDS.map((k) => (
                  <option key={k} value={k}>
                    {t(KIND_LABELS[k])}
                  </option>
                ))}
                {customKinds.map((k) => (
                  <option key={k.key} value={k.key}>
                    {k.name}
                  </option>
                ))}
                <option value={NEW_KIND}>{t("Add a type…")}</option>
              </select>
            </label>
            {newKind !== null && (
              <span className="documents-new-kind">
                <input
                  className="xc-input"
                  value={newKind}
                  maxLength={20}
                  autoFocus
                  aria-label={t("New type name")}
                  placeholder={t("For example: bank card")}
                  onChange={(e) => setNewKind(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      addKind();
                    }
                  }}
                />
                <button
                  type="button"
                  className="xc-btn small"
                  disabled={createKind.isPending}
                  onClick={addKind}
                >
                  {t("Add")}
                </button>
              </span>
            )}
            {newKind === null && removableKind && (
              <button
                type="button"
                className="xc-btn ghost small"
                disabled={deleteKind.isPending}
                onClick={removeKind}
              >
                {t("Delete this type")}
              </button>
            )}
          </div>
          <label className="xc-field">
            <span>{t("Name")}</span>
            <input
              className="xc-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              autoFocus
              placeholder="例如 李四的护照"
            />
          </label>
        </div>
        <div className="documents-form-row">
          <label className="xc-field">
            <span>{t("Holder")}</span>
            <input
              className="xc-input"
              value={holder}
              onChange={(e) => setHolder(e.target.value)}
              maxLength={200}
            />
          </label>
          <label className="xc-field">
            <span>{t("Document number")}</span>
            <input
              className="xc-input"
              value={number}
              onChange={(e) => setNumber(e.target.value)}
              maxLength={200}
              autoComplete="off"
            />
            <small>{t("Stored encrypted.")}</small>
          </label>
        </div>
        <div className="documents-form-row">
          <label className="xc-field">
            <span>{t("Issued or bought on")}</span>
            <input
              className="xc-input"
              type="date"
              value={issuedOn}
              onChange={(e) => setIssuedOn(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Expires on")}</span>
            <input
              className="xc-input"
              type="date"
              value={expiresOn}
              onChange={(e) => setExpiresOn(e.target.value)}
            />
            <small>{t("Leave empty if it never expires.")}</small>
          </label>
        </div>
        {isItem && (
          <div className="documents-form-row">
            <label className="xc-field">
              <span>{t("Price")}</span>
              <input
                className="xc-input"
                inputMode="decimal"
                value={price}
                onChange={(e) => setPrice(e.target.value)}
              />
            </label>
            <label className="xc-field documents-narrow">
              <span>{t("Currency")}</span>
              <select
                className="xc-select"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
              >
                {(CURRENCIES.includes(currency)
                  ? CURRENCIES
                  : [...CURRENCIES, currency]
                ).map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </label>
            <label className="xc-field">
              <span>{t("Serial number")}</span>
              <input
                className="xc-input"
                value={serial}
                onChange={(e) => setSerial(e.target.value)}
                maxLength={200}
              />
            </label>
          </div>
        )}
        <label className="xc-field">
          <span>{t("Remind before expiry (days)")}</span>
          <input
            className="xc-input"
            value={remind}
            onChange={(e) => setRemind(e.target.value)}
            inputMode="numeric"
          />
          <small>
            {t(
              "For example 90, 30, 7. You are also told on the day it expires.",
            )}
          </small>
        </label>
        <div className="xc-field">
          <span>{t("Document remarks")}</span>
          <MarkdownEditor
            label={t("Document notes")}
            value={notes}
            onChange={setNotes}
            minRows={3}
          />
        </div>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button type="submit" className="xc-btn primary" disabled={saving}>
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
