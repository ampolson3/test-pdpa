import { getRequestConfig } from "next-intl/server";
import { hasLocale } from "next-intl";
import { getMessages } from "@pdpa/i18n";
import { routing } from "./routing";

export default getRequestConfig(async ({ requestLocale }) => {
  const requested = await requestLocale;
  const locale = hasLocale(routing.locales, requested) ? requested : routing.defaultLocale;

  return {
    locale,
    messages: await getMessages(locale),
  };
});
