import { computed, ref } from 'vue';

import { createSharedComposable } from '@vueuse/core';

import { getMessages, type Locale } from './messages';

export const useSimpleLocale = createSharedComposable(() => {
  const currentLocale = ref<Locale>('en-US');

  const setSimpleLocale = (locale: Locale) => {
    currentLocale.value = locale;
  };

  const $t = computed(() => {
    const localeMessages = getMessages(currentLocale.value);
    return (key: string) => {
      return localeMessages[key] || key;
    };
  });
  return {
    $t,
    currentLocale,
    setSimpleLocale,
  };
});
