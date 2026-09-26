import { createContext, useContext } from "react";
import { translate } from "../lib/i18n";
import type { Language, Text } from "../types/domain";

export const LanguageContext = createContext<Language>("zh");

export const useLanguage = () => useContext(LanguageContext);

export const useT = () => {
  const language = useContext(LanguageContext);
  return (text: Text) => translate(language, text);
};
