import type { AnyPromiseFunction } from '@vben/types';

export type OptionsItem = {
  [name: string]: any;
  children?: OptionsItem[];
  disabled?: boolean;
  label?: string;
  value?: string;
};

export interface Props {
  title?: string;
  toolbar?: boolean;
  checkable?: boolean;

  search?: boolean;
  searchText?: string;

  /** Convert numeric values to strings. */
  numberToString?: boolean;
  /** Function that fetches the options. */
  api?: (arg?: any) => Promise<OptionsItem[] | Record<string, any>>;
  /** Arguments passed to the api function. */
  params?: Record<string, any>;
  /** Field name on the api response that holds the options array. */
  resultField?: string;
  /** Field name used as the option label. */
  labelField?: string;
  /** Field name holding child nodes (for hierarchical components). */
  childrenField?: string;
  /** Field name used as the option value. */
  valueField?: string;
  /** Prop name through which the wrapped component accepts options. */
  optionsPropName?: string;
  /** Whether to call the api immediately on mount. */
  immediate?: boolean;
  /** Re-fetch on every `visibleEvent` (instead of caching). */
  alwaysLoad?: boolean;
  /** Hook invoked before the api request (transform args / abort). */
  beforeFetch?: AnyPromiseFunction<any, any>;
  /** Hook invoked after the api request (transform response). */
  afterFetch?: AnyPromiseFunction<any, any>;
  /** Static options; also used as a fallback if the api returns empty. */
  options?: OptionsItem[];
  /** Slot name where the "loading" indicator is rendered. */
  loadingSlot?: string;
  /** Event that triggers a refetch (e.g. dropdown-open). */
  visibleEvent?: string;
  /** v-model prop name (default: modelValue; some components: value). */
  modelPropName?: string;
  /** Whether tree-style components expand all nodes by default. */
  treeDefaultExpandAll?: boolean;
}

export type TreeEmits = {
  optionsChange: [OptionsItem[]];
  search: [string];
};

export enum ToolbarEnum {
  SELECT_ALL,
  UN_SELECT_ALL,
  EXPAND_ALL,
  UN_EXPAND_ALL,
  CHECK_STRICTLY,
  CHECK_UN_STRICTLY,
}

export interface MenuInfo {
  key: ToolbarEnum;
}
