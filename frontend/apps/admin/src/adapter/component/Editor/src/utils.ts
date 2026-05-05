import { preferences } from '@vben/preferences';

/**
 * Whether the current theme preference is dark mode.
 */
export function isDarkMode() {
  return preferences.theme.mode === 'dark';
}
