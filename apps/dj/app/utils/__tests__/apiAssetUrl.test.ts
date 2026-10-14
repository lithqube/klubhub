import { describe, it, expect, afterEach } from 'vitest';
import { apiAssetUrl, setApiAssetResolver } from '../apiAssetUrl';

describe('apiAssetUrl', () => {
  afterEach(() => {
    setApiAssetResolver(null);
  });

  it('returns the path unchanged when no resolver is registered', () => {
    expect(apiAssetUrl('tracklists/img.png')).toBe('tracklists/img.png');
  });

  it('uses the registered resolver when present', () => {
    setApiAssetResolver((path) => `data:image/png;base64,${path}`);
    expect(apiAssetUrl('tracklists/img.png')).toBe('data:image/png;base64,tracklists/img.png');
  });

  it('falls back to the path when the resolver returns null', () => {
    setApiAssetResolver(() => null);
    expect(apiAssetUrl('tracklists/img.png')).toBe('tracklists/img.png');
  });

  it('setApiAssetResolver(null) restores pass-through behavior', () => {
    setApiAssetResolver(() => 'resolved');
    setApiAssetResolver(null);
    expect(apiAssetUrl('x.png')).toBe('x.png');
  });
});
