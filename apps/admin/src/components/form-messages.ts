"use client";

import { useEffect, useMemo, useState } from "react";
import { createTranslator, useLocale, useTranslations } from "next-intl";
import { getMessages } from "@pdpa/i18n";
import type { ErrorCode, FieldError, Language, RendererMessages } from "@pdpa/form-renderer";

const codes: ErrorCode[] = ["required", "invalid_type", "invalid_option", "out_of_range", "too_long", "invalid_date", "invalid_email", "unknown_question"];

type Translate = (key: string, values?: Record<string, number>) => string;

function build(t: Translate): RendererMessages {
  return {
    errors: Object.fromEntries(codes.map((c) => [c, t(`errors.${c}`)])) as Record<ErrorCode, string>,
    yes: t("yes"),
    no: t("no"),
    choose: t("choose"),
    required: t("required"),
    readOnly: t("readOnly"),
    score: (score: number, max: number) => t("score", { score, max }),
  };
}

/**
 * The FormRenderer's UI strings in the form's language: the page's own translations, or — when a form is
 * shown in the other language (builder preview, a response viewed in English) — that locale's messages,
 * loaded on demand.
 */
export function useRendererMessages(lang?: Language): RendererMessages {
  const pageLocale = useLocale() as Language;
  const t = useTranslations("formRenderer");
  const own = useMemo(() => build(t as unknown as Translate), [t]);
  const [other, setOther] = useState<{ lang: Language; messages: RendererMessages }>();
  const want = lang ?? pageLocale;
  useEffect(() => {
    if (want === pageLocale || other?.lang === want) return;
    let live = true;
    void getMessages(want).then((messages) => {
      if (!live) return;
      const tr = createTranslator({ locale: want, messages, namespace: "formRenderer" });
      setOther({ lang: want, messages: build(tr as unknown as Translate) });
    });
    return () => {
      live = false;
    };
  }, [want, pageLocale, other?.lang]);
  return want !== pageLocale && other?.lang === want ? other.messages : own;
}

/** The problem's code, if the error is a problem+json body. */
export function problemCode(e: unknown): string | undefined {
  return typeof e === "object" && e !== null && "code" in e ? String((e as { code: unknown }).code) : undefined;
}

/** Field errors of a 422 forms.invalid_answers problem, as the renderer takes them. */
export function answerErrors(e: unknown): FieldError[] | undefined {
  if (problemCode(e) !== "forms.invalid_answers") return undefined;
  const list = (e as { errors?: { field: string; code: string }[] }).errors ?? [];
  return list.map((x) => ({ question: x.field, code: x.code as ErrorCode }));
}
