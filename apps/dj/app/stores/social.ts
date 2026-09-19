import { defineStore } from 'pinia';
import { ref } from 'vue';
import { useUiStore } from './ui';
import type { SocialAccount, ScheduledPost, CreatePostRequest, EditPostRequest } from '../types/social';

export const useSocialStore = defineStore('social', () => {
  const posts = ref<ScheduledPost[]>([]);
  const account = ref<SocialAccount | null>(null);
  const loading = ref(false);
  const composePanelOpen = ref(false);
  const prefilledImageId = ref<string | null>(null);

  // Helper to convert snake_case object keys to camelCase
  function toCamelCase(str: string): string {
    return str.replace(/(_\w)/g, (match) => match[1].toUpperCase());
  }

  function mapSocialAccount(raw: any): SocialAccount {
    const mapped: any = {};
    for (const [key, value] of Object.entries(raw)) {
      const camelKey = toCamelCase(key);
      mapped[camelKey] = value;
    }
    return mapped as SocialAccount;
  }

  function mapScheduledPost(raw: any): ScheduledPost {
    const mapped: any = {};
    for (const [key, value] of Object.entries(raw)) {
      const camelKey = toCamelCase(key);
      mapped[camelKey] = value;
    }
    return mapped as ScheduledPost;
  }

  async function loadPosts(): Promise<void> {
    loading.value = true;
    try {
      const result = await $fetch<{ data: ScheduledPost[] }>('/api/v1/social/posts');
      posts.value = result.data ? result.data.map(mapScheduledPost) : [];
    } catch (e) {
      useUiStore().showError(String(e));
    } finally {
      loading.value = false;
    }
  }

  async function loadAccount(): Promise<void> {
    try {
      const result = await $fetch<{ data: SocialAccount | null }>('/api/v1/social/accounts');
      account.value = result.data ? mapSocialAccount(result.data) : null;
    } catch (e) {
      useUiStore().showError(String(e));
    }
  }

  async function createPost(req: CreatePostRequest): Promise<void> {
    const formData = new FormData();
    formData.append('post_type', req.postType);
    formData.append('caption', req.caption);
    formData.append('scheduled_at', req.scheduledAt);
    formData.append('timezone_name', req.timezoneName);
    if (req.imageId) {
      formData.append('image_id', req.imageId);
    }
    if (req.imageFile) {
      formData.append('image_file', req.imageFile);
    }
    // Add account_id: prefer request, then store
    if (req.accountId) {
      formData.append('account_id', req.accountId);
    } else if (account.value?.id) {
      formData.append('account_id', account.value.id);
    }
    try {
      await $fetch('/api/v1/social/posts', {
        method: 'POST',
        body: formData,
      });
      await loadPosts();
    } catch (e) {
      useUiStore().showError(String(e));
      throw e; // re-throw so caller knows it failed
    }
  }

  async function editPost(id: string, req: EditPostRequest): Promise<void> {
    try {
      const result = await $fetch<{ data: ScheduledPost }>(`/api/v1/social/posts/${id}`, {
        method: 'PUT',
        body: req,
      });
      const idx = posts.value.findIndex((p) => p.id === id);
      if (idx !== -1) {
        posts.value[idx] = result.data;
      }
    } catch (e) {
      useUiStore().showError(String(e));
    }
  }

  async function deletePost(id: string): Promise<void> {
    try {
      await $fetch(`/api/v1/social/posts/${id}`, { method: 'DELETE' });
      posts.value = posts.value.filter((p) => p.id !== id);
    } catch (e) {
      useUiStore().showError(String(e));
    }
  }

  async function retryPost(id: string): Promise<void> {
    try {
      await $fetch(`/api/v1/social/posts/${id}/retry`, { method: 'POST' });
      await loadPosts();
    } catch (e) {
      useUiStore().showError(String(e));
    }
  }

  async function downloadImage(id: string): Promise<Blob | null> {
    try {
      const blob = await $fetch<Blob>(`/api/v1/social/posts/${id}/image`, {
        responseType: 'blob',
      });
      return blob;
    } catch (e) {
      useUiStore().showError(String(e));
      return null;
    }
  }

  async function initiateOAuth(): Promise<string | null> {
    try {
      const result = await $fetch<{ url: string }>('/api/v1/social/auth/url');
      return result.url ?? null;
    } catch (e) {
      useUiStore().showError(String(e));
      return null;
    }
  }

  async function disconnectAccount(id: string): Promise<void> {
    try {
      await $fetch(`/api/v1/social/accounts/${id}`, { method: 'DELETE' });
      account.value = null;
    } catch (e) {
      useUiStore().showError(String(e));
    }
  }

  function openComposePanel(imageId?: string): void {
    composePanelOpen.value = true;
    prefilledImageId.value = imageId ?? null;
  }

  function closeComposePanel(): void {
    composePanelOpen.value = false;
    prefilledImageId.value = null;
  }

  return {
    posts,
    account,
    loading,
    composePanelOpen,
    prefilledImageId,
    loadPosts,
    loadAccount,
    createPost,
    editPost,
    deletePost,
    retryPost,
    downloadImage,
    initiateOAuth,
    disconnectAccount,
    openComposePanel,
    closeComposePanel,
  };
});