export default defineEventHandler(() => {
  return {
    dj_name: 'SAMPLE_RATE',
    logo_path: null,
    logo_position: 'top-left',
    custom_placeholder_path: null,
    preset: 'dark',
    bg_mode: 'solid',
    bg_value: null,
    visible_fields: ['title', 'artist', 'bpm', 'key', 'durationSecs'],
    max_tracks: 50,
    track_range_start: null,
    track_range_end: null,
  }
})
