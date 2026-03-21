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

  async function loadPosts(): Promise<void> {
    loading.value = true;
    try {
      const result = await $fetch<{ data: ScheduledPost[] }>('/api/v1/social/posts');
      posts.value = result.data;
    } catch (e) {
      useUiStore().showError(String(e));
    } finally {
      loading.value = false;
    }
  }

  async function loadAccount(): Promise<void> {
    try {
      const result = await $fetch<{ data: SocialAccount | null }>('/api/v1/social/accounts');
      account.value = result.data;
    } catch (e) {
      useUiStore().showError(String(e));
    }
  }

  async function createPost(req: CreatePostRequest): Promise<void> {
    try {
      const formData = new FormData();
      formData.append('postType', req.postType);
      formData.append('caption', req.caption);
      formData.append('scheduledAt', req.scheduledAt);
      formData.append('timezoneName', req.timezoneName);
      if (req.imageId) {
        formData.append('imageId', req.imageId);
      }
      if (req.imageFile) {
        formData.append('imageFile', req.imageFile);
      }
      await $fetch('/api/v1/social/posts', {
        method: 'POST',
        body: formData,
      });
      await loadPosts();
    } catch (e) {
      useUiStore().showError(String(e));
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
      const result = await $fetch<{ data: ScheduledPost }>(`/api/v1/social/posts/${id}/retry`, {
        method: 'POST',
      });
      const idx = posts.value.findIndex((p) => p.id === id);
      if (idx !== -1) {
        posts.value[idx] = result.data;
      }
    } catch (e) {
      useUiStore().showError(String(e));
    }
  }

  async function downloadImage(id: string): Promise<void> {
    try {
      const blob = await $fetch<Blob>(`/api/v1/social/posts/${id}/image`, {
        responseType: 'blob',
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `social-post-${id}.jpg`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      useUiStore().showError(String(e));
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
    disconnectAccount,
    openComposePanel,
    closeComposePanel,
  };
});
