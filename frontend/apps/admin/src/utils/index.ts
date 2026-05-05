/**
 * Deep-clone a value. Handles objects, arrays, Map, Set, Date, RegExp,
 * and circular references.
 */
export function deepClone<T>(value: T): T {
  // Prefer native structuredClone; fall back to a hand-rolled walk on error.
  if (typeof (globalThis as any).structuredClone === 'function') {
    try {
      return (globalThis as any).structuredClone(value);
    } catch {
      // structuredClone refuses certain inputs (Vue reactive proxies,
      // DOM nodes, window, etc.); fall through to the manual path.
    }
  }

  const seen = new WeakMap<any, any>();

  const _clone = (v: any): any => {
    if (v === null || typeof v !== 'object') return v;
    if (v instanceof Date) return new Date(v.getTime());
    if (v instanceof RegExp) return new RegExp(v.source, v.flags);
    if (v instanceof Map) {
      if (seen.has(v)) return seen.get(v);
      const m = new Map();
      seen.set(v, m);
      for (const [k, val] of v) m.set(_clone(k), _clone(val));
      return m;
    }
    if (v instanceof Set) {
      if (seen.has(v)) return seen.get(v);
      const s = new Set();
      seen.set(v, s);
      for (const item of v) s.add(_clone(item));
      return s;
    }
    if (seen.has(v)) return seen.get(v);

    if (Array.isArray(v)) {
      const arr: any[] = [];
      seen.set(v, arr);
      for (let i = 0; i < v.length; i++) arr[i] = _clone(v[i]);
      return arr;
    }

    const obj: any = Object.create(Object.getPrototypeOf(v));
    seen.set(v, obj);
    for (const key of Reflect.ownKeys(v)) {
      obj[key as any] = _clone((v as any)[key as any]);
    }
    return obj;
  };

  return _clone(value);
}

/**
 * Convert an integer cents amount to a "dollars.cents" string.
 * @param cents amount in cents
 */
export function centToDollar(cents: number): string {
  return (cents / 100).toFixed(2);
}

/**
 * Convert a byte count to a "x.xx GB" string.
 */
export function bytesToGB(bytes: number): string {
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

/**
 * Format a byte count into the most readable unit (B/KB/MB/GB/TB/PB).
 * @param bytes byte count
 * @param decimals number of fractional digits (default 2)
 * @returns e.g. "2.50 MB", "1.80 GB"
 */
export function formatBytes(bytes: number, decimals: number = 2): string {
  // Special case for zero — Math.log(0) is -Infinity.
  if (bytes === 0) return '0 B';

  // Use 1024 (binary) as the unit base.
  const k = 1024;
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

  // Pick the unit by exponent of 1024.
  const i = Math.floor(Math.log(bytes) / Math.log(k));

  // Clamp at the largest defined unit.
  const unitIndex = Math.min(i, units.length - 1);

  // Compute the numeric value for that unit, rounded to `decimals`.
  const value = (bytes / k ** unitIndex).toFixed(decimals);

  return `${value} ${units[unitIndex]}`;
}

/**
 * Extract all valid numbers (primitive `number`, not NaN, finite) from
 * an array. Wrapped Number objects, NaN, and ±Infinity are dropped.
 * @returns array containing only valid numbers
 */
export function filterNumbers(arr: unknown[]): number[] {
  if (!Array.isArray(arr)) {
    throw new TypeError('input must be an array');
  }

  const is_valid_number = (value: unknown): value is number => {
    return (
      typeof value === 'number' && // primitive `number`
      Object.prototype.toString.call(value) === '[object Number]' && // exclude `new Number()` wrapper
      !Number.isNaN(value) && // exclude NaN
      Number.isFinite(value) // exclude ±Infinity
    );
  };

  return arr.filter((element) => is_valid_number(element));
}
