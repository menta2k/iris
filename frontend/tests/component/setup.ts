import { config } from '@vue/test-utils'
import { vuetify } from '@/plugins/vuetify'

// jsdom lacks a few browser APIs Vuetify touches.
if (!window.matchMedia) {
  window.matchMedia = (query: string): MediaQueryList =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList
}

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}

if (!globalThis.visualViewport) {
  Object.defineProperty(globalThis, 'visualViewport', {
    value: new EventTarget(),
    writable: true,
  })
}

// The ui/* primitives render Vuetify components, so every mounted page
// needs the plugin installed.
config.global.plugins = [...(config.global.plugins ?? []), vuetify as never]
