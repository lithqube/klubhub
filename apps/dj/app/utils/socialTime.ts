/**
 * The `datetime-local` value of a UTC instant in the post's own time zone.
 * The API reads the field back as wall time in that zone, so seeding it with
 * UTC time would move the post by the zone's offset on every save.
 */
export function toZonedInput(utc: string, timeZone: string): string {
  const date = new Date(utc)
  if (Number.isNaN(date.getTime())) return ''
  try {
    return new Intl.DateTimeFormat('sv-SE', {
      timeZone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
    }).format(date).replace(' ', 'T')
  } catch {
    return '' // unknown zone: an empty field is rejected on save, a wrong time is not
  }
}
