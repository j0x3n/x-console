import type * as Model from "../../../types/domain";
import React from "react";

interface TemplateIconProps {
  template: Model.HireTemplate;
  large?: boolean;
}

export function TemplateIcon({ template, large = false }: TemplateIconProps) {
  return (
    <span
      className={`hire-template-icon ${template.id}${large ? " large" : ""}`}
      role="img"
      aria-label={template.name}
    >
      <svg viewBox="0 0 24 24" aria-hidden="true">
        {template.id === "relay" ? (
          <rect x="5" y="5" width="14" height="14" rx="3" />
        ) : template.id === "sentry" ? (
          <path d="M12 3 21 10 17.5 20H6.5L3 10Z" />
        ) : template.id === "tally" ? (
          <>
            <rect x="4" y="13" width="4" height="8" rx="1" />
            <rect x="10" y="8" width="4" height="13" rx="1" />
            <rect x="16" y="3" width="4" height="18" rx="1" />
          </>
        ) : (
          <path d="M10 3h4v7h7v4h-7v7h-4v-7H3v-4h7Z" />
        )}
      </svg>
    </span>
  );
}
