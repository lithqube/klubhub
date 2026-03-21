<script setup lang="ts">
import type { ScheduledPost } from '~/types/social'
import SocialPostCard from '~/components/social/SocialPostCard.vue'
import SocialPostCardFailed from '~/components/social/SocialPostCardFailed.vue'
import SocialNextSlotCard from '~/components/social/SocialNextSlotCard.vue'

defineProps<{
  posts: ScheduledPost[]
}>()

function isFailed(post: ScheduledPost): boolean {
  return post.status === 'failed' || post.status === 'permanently_failed'
}
</script>

<template>
  <div class="grid grid-cols-3 gap-4">
    <template v-for="post in posts" :key="post.id">
      <SocialPostCardFailed v-if="isFailed(post)" :post="post" />
      <SocialPostCard v-else :post="post" />
    </template>
    <SocialNextSlotCard />
  </div>
</template>
