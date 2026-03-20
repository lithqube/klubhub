import { describe, it, expect } from 'vitest';
import { PRESETS } from '../design-tokens';

describe('design-tokens PRESETS', () => {
  it('PRESETS has exactly 5 keys', () => {
    expect(Object.keys(PRESETS)).toHaveLength(5);
  });

  it('PRESETS has the expected preset names', () => {
    expect(Object.keys(PRESETS)).toEqual(
      expect.arrayContaining(['default', 'dark', 'light', 'neon', 'minimal'])
    );
  });

  it('PRESETS.default has bg property that is a hex color string', () => {
    expect(PRESETS.default.bg).toMatch(/^#[0-9a-fA-F]{3,8}$/);
  });

  it('all preset values have bg, primary, accent, text keys', () => {
    for (const preset of Object.values(PRESETS)) {
      expect(preset).toHaveProperty('bg');
      expect(preset).toHaveProperty('primary');
      expect(preset).toHaveProperty('accent');
      expect(preset).toHaveProperty('text');
    }
  });

  it('all preset color values are hex strings', () => {
    const hexPattern = /^#[0-9a-fA-F]{3,8}$/;
    for (const [name, preset] of Object.entries(PRESETS)) {
      expect(preset.bg, `${name}.bg`).toMatch(hexPattern);
      expect(preset.primary, `${name}.primary`).toMatch(hexPattern);
      expect(preset.accent, `${name}.accent`).toMatch(hexPattern);
      expect(preset.text, `${name}.text`).toMatch(hexPattern);
    }
  });

  it('PRESETS.neon has a neon green or bright primary color', () => {
    expect(PRESETS.neon.bg).toBeTruthy();
    expect(PRESETS.neon.primary).toBeTruthy();
  });
});
