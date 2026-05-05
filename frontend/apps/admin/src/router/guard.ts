import type { Router } from 'vue-router';

import { DEFAULT_HOME_PATH, LOGIN_PATH } from '@vben/constants';
import { preferences } from '@vben/preferences';
import { useAccessStore, useUserStore } from '@vben/stores';
import { startProgress, stopProgress } from '@vben/utils';

import { accessRoutes, coreRouteNames } from '#/router/routes';
import { useAuthStore } from '#/stores';

import { generateAccess } from './access';

/**
 * Generic guards (loading-state tracking, top progress bar).
 */
function setupCommonGuard(router: Router) {
  // Track which routes have already been loaded so transition effects
  // don't replay on every revisit.
  const loadedPaths = new Set<string>();

  router.beforeEach((to) => {
    to.meta.loaded = loadedPaths.has(to.path);

    // Top-bar progress indicator on first navigation to a route.
    if (!to.meta.loaded && preferences.transition.progress) {
      startProgress();
    }
    return true;
  });

  router.afterEach((to) => {
    // Mark the route as loaded so subsequent navigations skip the
    // initial-load transition.
    loadedPaths.add(to.path);

    // Stop the top-bar progress indicator.
    if (preferences.transition.progress) {
      stopProgress();
    }
  });
}

/**
 * Access-control guard.
 */
function setupAccessGuard(router: Router) {
  router.beforeEach(async (to, from) => {
    const accessStore = useAccessStore();
    const userStore = useUserStore();
    const authStore = useAuthStore();

    // Core routes (login, error pages) bypass the auth check.
    if (coreRouteNames.includes(to.name as string)) {
      if (to.path === LOGIN_PATH && accessStore.accessToken) {
        return decodeURIComponent(
          (to.query?.redirect as string) ||
            userStore.userInfo?.homePath ||
            DEFAULT_HOME_PATH,
        );
      }
      return true;
    }

    // Access-token check.
    if (!accessStore.accessToken) {
      // Routes that explicitly opt out of the access check are allowed.
      if (to.meta.ignoreAccess) {
        return true;
      }

      // No token: redirect to the login page, preserving the target so
      // the user lands back where they were after login.
      if (to.fullPath !== LOGIN_PATH) {
        return {
          path: LOGIN_PATH,
          // Drop the query if we're already at the home path.
          query:
            to.fullPath === DEFAULT_HOME_PATH
              ? {}
              : { redirect: encodeURIComponent(to.fullPath) },
          // Carry the current target through so login can redirect back.
          replace: true,
        };
      }
      return to;
    }

    // Have we already generated the dynamic route table?
    if (accessStore.isAccessChecked) {
      return true;
    }

    // Generate the route table from the user's permission codes.
    const userPermissionCodes = await authStore.getUserPermissionCodes();
    if (!userPermissionCodes) {
      return false;
    }

    // Build the menu + route tree the user is allowed to see.
    const { accessibleMenus, accessibleRoutes } = await generateAccess({
      roles: userPermissionCodes,
      router,
      // Routes the user lacks permission for stay visible in the menu
      // but redirect to 403 on click.
      routes: accessRoutes,
    });

    // Persist the resolved menu + routes.
    accessStore.setAccessMenus(accessibleMenus);
    accessStore.setAccessRoutes(accessibleRoutes);
    accessStore.setIsAccessChecked(true);

    const redirectPath = (from.query.redirect ??
      (to.path === DEFAULT_HOME_PATH
        ? userStore.userInfo?.homePath || DEFAULT_HOME_PATH
        : to.fullPath)) as string;

    return {
      ...router.resolve(decodeURIComponent(redirectPath)),
      replace: true,
    };
  });
}

/**
 * Install all router guards for the project.
 */
function createRouterGuard(router: Router) {
  /** Generic. */
  setupCommonGuard(router);
  /** Access control. */
  setupAccessGuard(router);
}

export { createRouterGuard };
