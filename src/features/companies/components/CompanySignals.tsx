import type * as Model from "../../../types/domain";
import React from "react";
import AccountCard from "./AccountCard";
import AccountIcon from "./AccountIcon";
import { FileText } from "lucide-react";

interface CompanySignalsProps {
  L: Model.Localize;
  signals: Model.AccountSignal[];
}

export default function CompanySignals({ L, signals }: CompanySignalsProps) {
  return (
    <AccountCard title={L("Signals", "信号")} count={signals.length}>
      <div className="ws-account-list">
        {signals.map((item, index) => (
          <div className="ws-account-signal-row" key={index}>
            <i className={item[2]} />
            <div>
              <b>{L(item[0], item[1])}</b>
              <small>
                <AccountIcon name={item[3]} />
                {item[3]} <FileText size={12} />
                {item[4]}
              </small>
            </div>
            <time>{item[5]}</time>
          </div>
        ))}
      </div>
    </AccountCard>
  );
}
