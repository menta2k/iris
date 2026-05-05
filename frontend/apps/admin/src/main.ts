import { initPreferences } from '@vben/preferences';
import { unmountGlobalLoading } from '@vben/utils';

import { overridesPreferences } from './preferences';

/**
 * Wait for app initialization to finish before mounting and rendering.
 */
async function initApplication() {
  // The namespace identifies this project; it's used as a prefix for
  // user-preference keys, storage keys, and any other state that must be
  // isolated from other apps sharing the same browser origin.
  const env = import.meta.env.PROD ? 'prod' : 'dev';
  const appVersion = import.meta.env.VITE_APP_VERSION;
  const namespace = `${import.meta.env.VITE_APP_NAMESPACE}-${appVersion}-${env}`;

  // Initialize app preferences.
  await initPreferences({
    namespace,
    overrides: overridesPreferences,
  });

  // Boot the Vue app — main logic and views.
  const { bootstrap } = await import('./bootstrap');
  await bootstrap(namespace);

  // Tear down the global loading splash.
  unmountGlobalLoading();
}

initApplication();
