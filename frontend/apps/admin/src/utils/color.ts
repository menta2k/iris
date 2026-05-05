/**
 * Deterministic per-string colour (HSL with mid-range saturation +
 * lightness so labels stay readable).
 */
export const getRandomColor = (str: string) => {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    hash = str.charCodeAt(i) + ((hash << 5) - hash);
  }
  const hue = Math.abs(hash % 360);
  return `hsl(${hue}, 50%, 85%)`;
};

/**
 * Deterministic colour for a single character (e.g. avatar initial).
 */
export const getCharColor = (char: string) => {
  let hash = 0;
  for (let i = 0; i < char.length; i++) {
    hash = char.charCodeAt(i) + ((hash << 5) - hash);
  }
  const hue = Math.abs(hash % 360);
  const saturation = 60;
  const lightness = 45;
  return `hsl(${hue}, ${saturation}%, ${lightness}%)`;
};
