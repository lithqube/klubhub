import { findEvent } from '../../../-mockDb'
const stamp = (iso: string) => iso.replace(/[-:]/g, '').replace(/\.\d{3}/, '')
export default defineEventHandler((event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  setHeader(event, 'Content-Type', 'text/calendar; charset=utf-8')
  return ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//KlubHub//Promoter//EN', 'BEGIN:VEVENT', `UID:${e.id}@promoter.klubhub`,
    `SEQUENCE:${e.version - 1}`, `DTSTAMP:${stamp(new Date().toISOString())}`, `DTSTART:${stamp(e.starts_at)}`, `DTEND:${stamp(e.ends_at)}`,
    `SUMMARY:${e.title}`, `LOCATION:${e.city}`, 'END:VEVENT', 'END:VCALENDAR', ''].join('\r\n')
})
