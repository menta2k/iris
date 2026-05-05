import { defineOverridesPreferences } from '@vben/preferences';

/**
 * Project-level preference overrides.
 * Only override what differs from the Vben defaults.
 */
export const overridesPreferences = defineOverridesPreferences({
  app: {
    name: import.meta.env.VITE_APP_TITLE,
    accessMode: import.meta.env.VITE_ROUTER_ACCESS_MODE,
    locale: 'en-US',
  },
  copyright: {
    companyName: 'Iris',
    companySiteLink: '',
    date: '2026',
    enable: true,
    icp: '',
    icpLink: '',
    settingShow: false,
  },
});
