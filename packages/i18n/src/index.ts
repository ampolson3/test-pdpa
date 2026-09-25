export const locales = ["th", "en"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "th";

export async function getMessages(locale: Locale) {
  switch (locale) {
    case "en":
      return (await import("./locales/en.json")).default;
    default:
      return (await import("./locales/th.json")).default;
  }
}
