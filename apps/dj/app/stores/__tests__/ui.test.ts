import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useUiStore } from '../ui';

describe('useUiStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('initial step is "upload"', () => {
    const store = useUiStore();
    expect(store.step).toBe('upload');
  });

  it('initial uploadLoading is false', () => {
    const store = useUiStore();
    expect(store.uploadLoading).toBe(false);
  });

  it('initial uploadError is null', () => {
    const store = useUiStore();
    expect(store.uploadError).toBeNull();
  });

  it('initial editLoading is false', () => {
    const store = useUiStore();
    expect(store.editLoading).toBe(false);
  });

  it('after setting uploadError = "fail", clearErrors() sets it back to null', () => {
    const store = useUiStore();
    store.uploadError = 'fail';
    expect(store.uploadError).toBe('fail');
    store.clearErrors();
    expect(store.uploadError).toBeNull();
  });

  it('after setting editError = "oops", clearErrors() clears it', () => {
    const store = useUiStore();
    store.editError = 'oops';
    store.clearErrors();
    expect(store.editError).toBeNull();
  });

  it('clearErrors() clears all error fields', () => {
    const store = useUiStore();
    store.uploadError = 'e1';
    store.editError = 'e2';
    store.exportError = 'e3';
    store.pastTracklistsError = 'e4';
    store.clearErrors();
    expect(store.uploadError).toBeNull();
    expect(store.editError).toBeNull();
    expect(store.exportError).toBeNull();
    expect(store.pastTracklistsError).toBeNull();
  });
});
