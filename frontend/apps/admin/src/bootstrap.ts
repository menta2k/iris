import { createApp, watchEffect } from 'vue';

import { registerAccessDirective } from '@vben/access';
import { preferences } from '@vben/preferences';
import { initStores } from '@vben/stores';
import '@vben/styles';
import '@vben/styles/antd';

import { useTitle } from '@vueuse/core';

import { $t, setupI18n } from '#/locales';

import { initComponentAdapter } from './adapter/component';
import App from './app.vue';
import { registerGlobComp } from './registerGlobComp';
import { router } from './router';

async function bootstrap(namespace: string) {
  // Initialize component adapters (form / table / etc.).
  await initComponentAdapter();

  const app = createApp(App);

  // Register globally-available components.
  registerGlobComp(app);

  // i18n setup.
  await setupI18n(app);

  // Pinia store setup.
  await initStores(app, { namespace });

  // Install the access-control directive.
  registerAccessDirective(app);

  // Wire the router (incl. guards).
  app.use(router);

  // Dynamically update the document title from the current route.
  watchEffect(() => {
    if (preferences.app.dynamicTitle) {
      const routeTitle = router.currentRoute.value.meta?.title;
      const pageTitle =
        (routeTitle ? `${$t(routeTitle)} - ` : '') + preferences.app.name;
      useTitle(pageTitle);
    }
  });

  app.mount('#app');
}

export { bootstrap };
