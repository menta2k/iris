import type { LocaleSetupOptions, SupportedLanguagesType } from '@vben/locales';
import type { Locale } from 'ant-design-vue/es/locale';

import type { App } from 'vue';
import { ref } from 'vue';

import {
  $t,
  setupI18n as coreSetup,
  loadLocalesMapFromDir,
} from '@vben/locales';
import { preferences } from '@vben/preferences';

import antdEnLocale from 'ant-design-vue/es/locale/en_US';
import antdBgLocale from 'ant-design-vue/es/locale/bg_BG';
import dayjs from 'dayjs';

// en-US is the default. The vendored Vben SupportedLanguagesType has
// been patched to ('bg-BG' | 'en-US') in packages/locales/src/typing.ts
// so this file no longer needs to widen the type.
const antdLocale = ref<Locale>(antdEnLocale);

const modules = import.meta.glob('./langs/**/*.json');

const localesMap = loadLocalesMapFromDir(
  /\.\/langs\/([^/]+)\/(.*)\.json$/,
  modules,
);

/**
 * Load the application's locale messages. Could later be swapped for a
 * server-fetched bundle if the catalog grows.
 */
async function loadMessages(lang: SupportedLanguagesType) {
  const [appLocaleMessages] = await Promise.all([
    localesMap[lang]?.(),
    loadThirdPartyMessage(lang),
  ]);
  return appLocaleMessages?.default;
}

/**
 * Load locale messages for third-party libraries (ant-design-vue, dayjs).
 */
async function loadThirdPartyMessage(lang: SupportedLanguagesType) {
  await Promise.all([loadAntdLocale(lang), loadDayjsLocale(lang)]);
}

/**
 * Load the dayjs locale bundle for the current language.
 */
async function loadDayjsLocale(lang: SupportedLanguagesType) {
  let locale;
  switch (lang) {
    case 'en-US': {
      locale = await import('dayjs/locale/en');
      break;
    }
    case 'bg-BG': {
      locale = await import('dayjs/locale/bg');
      break;
    }
    // Default to English for any unknown locale.
    default: {
      locale = await import('dayjs/locale/en');
    }
  }
  if (locale) {
    dayjs.locale(locale);
  } else {
    console.error(`Failed to load dayjs locale for ${lang}`);
  }
}

/**
 * Load the ant-design-vue locale bundle for the current language.
 */
async function loadAntdLocale(lang: SupportedLanguagesType) {
  switch (lang) {
    case 'en-US': {
      antdLocale.value = antdEnLocale;
      break;
    }
    case 'bg-BG': {
      antdLocale.value = antdBgLocale;
      break;
    }
  }
}

async function setupI18n(app: App, options: LocaleSetupOptions = {}) {
  await coreSetup(app, {
    defaultLocale: preferences.app.locale,
    loadMessages,
    missingWarn: !import.meta.env.PROD,
    ...options,
  });
}

export { $t, antdLocale, setupI18n };
