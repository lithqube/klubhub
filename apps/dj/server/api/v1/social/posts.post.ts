export default defineEventHandler(async () => {
  // In mock mode, return a new draft post so the store refreshes correctly
  return {
    data: {
      id: `sp-mock-${Date.now()}`,
      accountId: 'acc-001',
      status: 'scheduled',
      postType: 'feed',
      caption: '',
      imageMinioPath: '',
      scheduledAtUtc: new Date(Date.now() + 86400000).toISOString(),
      timezoneName: 'Europe/Berlin',
      retryCount: 0,
      nextRetryAt: null,
      lastError: '',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    },
  }
})
