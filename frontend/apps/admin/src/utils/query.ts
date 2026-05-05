/**
 * Query operators understood by the backend's list filters.
 */
export type QueryOperator =
  | 'contains'
  | 'endswith'
  | 'eq'
  | 'exact'
  | 'gt'
  | 'gte'
  | 'icontains'
  | 'iendswith'
  | 'iexact'
  | 'in'
  | 'iregex'
  | 'isnull'
  | 'istartswith'
  | 'lt'
  | 'lte'
  | 'ne'
  | 'nin'
  | 'not'
  | 'not_in'
  | 'not_isnull'
  | 'range'
  | 'regex'
  | 'search'
  | 'startswith';

/**
 * Date/time operators (component extractors on a timestamp column).
 */
export type DateOperator =
  | 'date'
  | 'day'
  | 'iso_week_day'
  | 'iso_year'
  | 'minute'
  | 'month'
  | 'quarter'
  | 'second'
  | 'time'
  | 'week'
  | 'week_day'
  | 'year';

/**
 * Leaf-level condition: keys are `field` or `field__operator`; values
 * are scalars (or arrays of scalars for `in`-style operators).
 */
export type BaseQueryCondition = Record<
  string,
  (number | string)[] | boolean | number | string
>;

/**
 * Logical-combination node: supports nested $and / $or.
 */
export interface LogicalQueryNode {
  $and?: (BaseQueryCondition | LogicalQueryNode)[];
  $or?: (BaseQueryCondition | LogicalQueryNode)[];
}

/**
 * Top-level Query type: accepts a flat array of conditions OR an
 * arbitrary nested logical tree.
 *
 * @example
 * const complexQuery: QueryRule = {
 *   $and: [
 *     { deptId: 1 },
 *     {
 *       $or: [
 *         { entryTime__gte: "2024-01-01" },
 *         { userName__icontains: "alice" }
 *       ]
 *     },
 *     { status: "active" }
 *   ]
 * };
 */
export type QueryRule =
  | (BaseQueryCondition | LogicalQueryNode)[]
  | LogicalQueryNode;

/**
 * Returns true for plain objects (excludes class instances, Date, etc.).
 */
function isPlainObject(v: any): v is Record<string, any> {
  return Object.prototype.toString.call(v) === '[object Object]';
}

/**
 * Strip null / undefined / empty-string values from a query rule.
 * Recurses into nested arrays and plain objects; preserves Dates,
 * RegExps, and class instances as-is.
 */
export function cleanQueryRule(obj: any): any {
  if (obj === null || obj === undefined || obj === '') {
    return undefined;
  }

  const t = typeof obj;
  if (t === 'number' || t === 'boolean' || t === 'string') {
    return obj;
  }

  if (Array.isArray(obj)) {
    const arr = obj
      .map((v) => cleanQueryRule(v))
      .filter((v) => v !== undefined);
    return arr.length === 0 ? undefined : arr;
  }

  // Only recurse into plain objects; Date/RegExp/class-instance values
  // are passed through as-is.
  if (isPlainObject(obj)) {
    const entries = Object.entries(obj)
      .map(([k, v]) => [k, cleanQueryRule(v)] as [string, any])
      .filter(([_, v]) => v !== undefined);
    const result = Object.fromEntries(entries);
    return Object.keys(result).length === 0 ? undefined : result;
  }

  // Non-plain objects (Date / RegExp / instances) — pass through.
  return obj;
}

/**
 * Drop null, undefined, and empty-string entries from an object.
 */
export const removeNullUndefined = (obj: any) =>
  Object.fromEntries(
    Object.entries(obj).filter(
      ([_, v]) => v !== null && v !== undefined && v !== '',
    ),
  );

/**
 * Build a JSON filter string for a list endpoint.
 * @param formValues form values from the search form
 * @param needCleanTenant strip tenant fields (tenant_id / tenantId)
 */
export function makeQueryString(
  formValues?: null | object,
  needCleanTenant: boolean = false,
): string | undefined {
  if (formValues === null || formValues === undefined) {
    return undefined;
  }

  // Strip empty values.
  const cleaned: any = removeNullUndefined(formValues);

  if (cleaned === undefined) return undefined;

  // If it's already an array, use the array path.
  if (Array.isArray(cleaned)) {
    return cleaned.length === 0 ? undefined : JSON.stringify(cleaned);
  }

  // Drop now-empty objects.
  if (Object.keys(cleaned).length === 0) {
    return undefined;
  }

  if (needCleanTenant) {
    // Strip the tenant fields (both snake_case + camelCase forms).
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { tenant_id, tenantId, ...rest } = cleaned as Record<string, any>;

    // Drop now-empty objects.
    if (Object.keys(rest).length === 0) {
      return undefined;
    }

    return JSON.stringify(rest);
  }

  // Default: return the cleaned object as JSON.
  return JSON.stringify(cleaned);
}

/**
 * Build a Google AIP-160 filter string for a list endpoint.
 */
export function makeFilterString(
  filterValues?: null | object,
): string | undefined {
  if (filterValues === null || filterValues === undefined) {
    return undefined;
  }

  // Strip empty values.
  filterValues = removeNullUndefined(filterValues);
}

/**
 * Build the order-by query parameter (defaults to newest first).
 */
export function makeOrderBy(orderBy?: null | string[]): string | undefined {
  if (orderBy === undefined || orderBy === null) {
    orderBy = ['-created_at'];
  }
  return JSON.stringify(orderBy) ?? undefined;
}

/**
 * Build an update field-mask (comma-separated field names).
 */
export function makeUpdateMask(keys: string[]): string {
  if (keys === undefined || keys.length === 0) {
    return '';
  }
  return keys.join(',');
}

/**
 * Return a copy of the object with the given keys removed.
 *
 * @example
 * const original = { a: 1, b: 2, c: 3 };
 * const result = omit(original, ['b', 'c']); // { a: 1 }
 *
 * @param obj source object
 * @param keys key (or keys) to omit
 */
export function omit<T extends Record<string, any>, K extends string>(
  obj: null | T | undefined,
  keys: K | K[],
): Omit<T, K> {
  if (obj === null || typeof obj !== 'object') return obj as any;
  const result = { ...obj } as Record<string, any>;
  const keysArr = Array.isArray(keys) ? keys : [keys];
  for (const key of keysArr) {
    if (Object.prototype.hasOwnProperty.call(result, key)) {
      // eslint-disable-next-line @typescript-eslint/no-dynamic-delete
      delete result[key];
    }
  }
  return result as Omit<T, K>;
}
