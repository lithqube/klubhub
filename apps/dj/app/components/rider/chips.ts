// Quick-insert chip constants for the four rider sections. Editable by
// the user underneath via free-text; chips are the one-tap start.

export const TECH_CHIPS: string[] = [
  '2× CDJ-3000',
  'DJM-A9 or better',
  'Monitor wedges ×2',
  'XLR line out',
  'Own controller allowed',
  'Booth PA check on arrival',
]

export const HOSPITALITY_CHIPS: string[] = [
  '4× bottled water',
  '2× hot meals',
  'Soft drinks',
  'Towels',
  'Green room access',
]

export const BACKLINE_CHIPS: string[] = [
  'DJM-A9 mixer',
  'Pioneer XDJ setup',
  'Technics SL-1200',
  'No rotary mixers',
]

export const OTHER_CHIPS: string[] = [
  'Arrival 2h before doors',
  'No flash photography',
  'Load-in via stage door',
]

export const SECTION_CHIPS: Record<string, string[]> = {
  technical: TECH_CHIPS,
  hospitality: HOSPITALITY_CHIPS,
  backline: BACKLINE_CHIPS,
  otherNotes: OTHER_CHIPS,
}