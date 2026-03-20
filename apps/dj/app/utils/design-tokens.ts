/**
 * Image generation preset colors.
 *
 * These are SEPARATE from the UI theme tokens in styles.css.
 * These values are burned into exported tracklist images, not used for the application chrome.
 */

export type PresetKey = 'default' | 'dark' | 'light' | 'neon' | 'minimal';

export interface PresetColors {
  bg: string;
  primary: string;
  accent: string;
  text: string;
}

export const PRESETS: Record<PresetKey, PresetColors> = {
  default: {
    bg: '#1a1a2e',
    primary: '#e94560',
    accent: '#0f3460',
    text: '#ffffff',
  },
  dark: {
    bg: '#0a0a0a',
    primary: '#ffffff',
    accent: '#333333',
    text: '#cccccc',
  },
  light: {
    bg: '#f5f5f5',
    primary: '#1a1a2e',
    accent: '#e0e0e0',
    text: '#333333',
  },
  neon: {
    bg: '#0d0d0d',
    primary: '#00ff88',
    accent: '#ff0080',
    text: '#ffffff',
  },
  minimal: {
    bg: '#ffffff',
    primary: '#000000',
    accent: '#888888',
    text: '#000000',
  },
};
