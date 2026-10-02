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
  // In-memory recovery survives compose unmounts, not browser reloads.
  // List refreshes and CLOSE must never discard an already-created post ID.
  const pendingComposeUpload = ref<(CreatePostRequest & {
    postId: string | null; imageFile: File | null; error: string;
  }) | null>(null);

  const composeSubmitting = ref(false);

  // The store owns the entire operation, including the gap before creation
  // resolves. Component unmounts cannot drop the metadata, file or returned ID.
  async function submitCompose(req: CreatePostRequest, file: File | null): Promise<boolean> {
    if (composeSubmitting.value || (pendingComposeUpload.value?.postId && !file)) return false;
    composeSubmitting.value = true;
    if (!pendingComposeUpload.value) {
      pendingComposeUpload.value = { ...req, postId: null, imageFile: file, error: '' };
    }
    const pending = pendingComposeUpload.value;
    if (!pending.postId) Object.assign(pending, req);
    pending.imageFile = file;
    pending.error = '';
    try {
      if (!pending.postId) {
        await createPost(req, (id) => { pending.postId = id; });
      }
      if (pending.imageFile) await uploadImage(pending.postId!, pending.imageFile);
      pendingComposeUpload.value = null;
      closeComposePanel();
      return true;
    } catch (error) {
      if (pending.postId && isNotFound(error)) {
        pending.postId = null;
        pending.error = 'The incomplete post no longer exists. Review the metadata and schedule again.';
        return false;
      }
      pending.error = pending.postId
        ? `Post created, but image upload failed. Retry the image upload; do not create another post. ${String(error)}`
        : `Post creation failed. ${String(error)}`;
      return false; // UI handlers must not leave an unhandled rejection on unmount.
    } finally {
      composeSubmitting.value = false;
    }
  }

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

  async function createPost(req: CreatePostRequest, onCreated?: (id: string) => void): Promise<{ id: string }> {
    const formData = new FormData();
    formData.append('post_type', req.postType);
    formData.append('caption', req.caption);
    formData.append('scheduled_at', req.scheduledAt);
    formData.append('timezone_name', req.timezoneName);
    // Only attach the image_id when the caller is using the legacy
    // "reuse existing object" flow (no upload happens server-side).
    if (req.imageId) {
      formData.append('image_id', req.imageId);
    }
    // Add account_id: prefer request, then store
    if (req.accountId) {
      formData.append('account_id', req.accountId);
    } else if (account.value?.id) {
      formData.append('account_id', account.value.id);
    }
    // The response is the full { data: ScheduledPost } envelope.
    // Return the new post id so callers can immediately call
    // uploadImage(postId, file) when the caller needs to attach a
    // new image file as a second request (see C.1).
    const result = await $fetch<{ data: ScheduledPost }>('/api/v1/social/posts', {
      method: 'POST',
      body: formData,
    });
    const created = mapScheduledPost(result.data);
    onCreated?.(created.id); // retain identity before any refresh can yield
    // Refresh the local list so the new one shows up immediately.
    await loadPosts();
    return { id: created.id };
  }

  // uploadImage attaches an image file to an existing post via the
  // dedicated POST /posts/{id}/image endpoint. C.1 split this off
  // from createPost because (a) the server-side pipeline is validate
  // → upload → DB-update, and (b) the FE now wants to surface upload
  // progress / errors independently of post creation.
  async function uploadImage(postId: string, file: File): Promise<string> {
    const formData = new FormData();
    formData.append('image_file', file);
    const result = await $fetch<{ data: { path: string } }>(
      `/api/v1/social/posts/${postId}/image`,
      { method: 'POST', body: formData },
    );
    // Refresh so the updated imageStorageKey shows up in the list.
    await loadPosts();
    return result.data.path;
  }

  async function editPost(id: string, req: EditPostRequest): Promise<void> {
    try {
      const result = await $fetch<{ data: ScheduledPost }>(`/api/v1/social/posts/${id}`, {
        method: 'PUT',
        body: req,
      });
      const idx = posts.value.findIndex((p) => p.id === id);
      if (idx !== -1) {
        posts.value[idx] = mapScheduledPost(result.data);
      }
    } catch (e) {
      useUiStore().showError(String(e));
    }
  }

  function isNotFound(error: unknown): boolean {
    const e = error as { status?: number; statusCode?: number; response?: { status?: number } };
    return (e?.statusCode ?? e?.status ?? e?.response?.status) === 404;
  }

  async function deletePost(id: string): Promise<boolean> {
    try {
      await $fetch(`/api/v1/social/posts/${id}`, { method: 'DELETE' });
    } catch (e) {
      // A 404 confirms there is no active post to abandon. Other errors must
      // retain recovery: clearing it would silently leave a scheduled post.
      if (!isNotFound(e)) {
        useUiStore().showError(String(e));
        if (pendingComposeUpload.value?.postId === id) pendingComposeUpload.value.error = `Could not delete the incomplete post. Recovery retained. ${String(e)}`;
        return false;
      }
    }
    posts.value = posts.value.filter((p) => p.id !== id);
    if (pendingComposeUpload.value?.postId === id) pendingComposeUpload.value = null;
    return true;
  }

  async function discardCompose(): Promise<boolean> {
    if (composeSubmitting.value) return false;
    const pending = pendingComposeUpload.value;
    if (pending?.postId) {
      composeSubmitting.value = true;
      try {
        if (!await deletePost(pending.postId)) return false;
      } finally {
        composeSubmitting.value = false;
      }
    } else {
      pendingComposeUpload.value = null;
    }
    closeComposePanel();
    return true;
  }

  async function retryPost(id: string): Promise<void> {
    try {
      await $fetch(`/api/v1/social/posts/${id}/retry`, { method: 'POST' });
      await loadPosts();
    } catch (e) {
      useUiStore().showError(String(e));
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
    pendingComposeUpload,
    composeSubmitting,
    submitCompose,
    discardCompose,
    loadPosts,
    loadAccount,
    createPost,
    uploadImage,
    editPost,
    deletePost,
    retryPost,
    initiateOAuth,
    disconnectAccount,
    openComposePanel,
    closeComposePanel,
  };
});