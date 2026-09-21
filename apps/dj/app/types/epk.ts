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
  /** Signed display URL per photoPaths entry. */
  photoUrls?: Record<string, string>
  sectionVisibility: SectionVisibility
  createdAt: string
  updatedAt: string
}

export interface EPKExport {
  id: string
  minioPath: string
  createdAt: string
}

export interface EPKExportCreateResult {
  id: string
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
