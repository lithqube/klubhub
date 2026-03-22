export interface PressQuote {
  text: string
  source: string
}

export type SectionVisibility = Record<string, boolean>

export interface EPKContent {
  id: string
  bioShort: string
  bioLong: string
  techRider: string
  stagePlotPath: string
  gigHighlights: string[]
  pressQuotes: PressQuote[]
  photoPaths: string[]
  sectionVisibility: SectionVisibility
  createdAt: string
  updatedAt: string
}

export interface EPKExport {
  id: string
  minioPath: string
  downloadUrl: string
  createdAt: string
}

export interface UpdateEPKContentRequest {
  bioShort?: string
  bioLong?: string
  techRider?: string
  stagePlotPath?: string
  gigHighlights?: string[]
  pressQuotes?: PressQuote[]
  photoPaths?: string[]
  sectionVisibility?: SectionVisibility
}
