// TypeScript types mirroring api/internal/rider/model.go. Used by the
// Pinia store, composables, and components.

export interface RiderTemplate {
  id: string
  name: string
  technical: string
  hospitality: string
  backline: string
  otherNotes: string
  createdAt: string
  updatedAt: string
}

export interface RiderAttachment {
  id: string
  gigId: string
  templateId: string | null
  technical: string
  hospitality: string
  backline: string
  otherNotes: string
  createdAt: string
  updatedAt: string
}

export interface RiderPdfExport {
  id: string
  downloadUrl: string
  createdAt: string
}

export interface RiderTemplateCreateInput {
  name: string
  technical?: string
  hospitality?: string
  backline?: string
  otherNotes?: string
}

export interface RiderTemplateUpdateInput {
  name?: string
  technical?: string
  hospitality?: string
  backline?: string
  otherNotes?: string
}

export interface RiderAttachmentCreateInput {
  gigId: string
  templateId?: string | null
  technical?: string
  hospitality?: string
  backline?: string
  otherNotes?: string
}

export interface RiderAttachmentUpdateInput {
  technical?: string
  hospitality?: string
  backline?: string
  otherNotes?: string
}

/**
 * The server answered 409 to an update: the record changed since this client
 * read it (another tab or device saved first). The update was NOT applied.
 */
export class RiderConflictError extends Error {
  constructor(
    public readonly kind: 'template' | 'attachment',
    public readonly id: string,
  ) {
    super(`The ${kind} was changed elsewhere.`)
    this.name = 'RiderConflictError'
  }
}

export type RiderSection = 'technical' | 'hospitality' | 'backline' | 'otherNotes'
export const RIDER_SECTIONS: readonly RiderSection[] = ['technical', 'hospitality', 'backline', 'otherNotes']

export const RIDER_SECTION_LABELS: Record<RiderSection, string> = {
  technical: 'TECHNICAL',
  hospitality: 'HOSPITALITY',
  backline: 'BACKLINE',
  otherNotes: 'OTHER NOTES',
}

// Rider status pill text — matches the COMPACT UX described in
// 04.5-05-PLAN.md §RiderAttachmentCard.
export function riderStatusLabel(
  attachment: RiderAttachment | null,
  template: RiderTemplate | null,
): { label: string; tone: 'muted' | 'accent' | 'amber' } {
  if (!attachment) {
    return { label: 'NO RIDER ATTACHED', tone: 'muted' }
  }
  // Override = the per-gig copy's content differs from its template. Not
  // timestamps: autosave rewrites all four sections, and an export or an
  // edit that restores the template text would otherwise read as "custom".
  const overridden =
    attachment.templateId !== null && template !== null &&
    RIDER_SECTIONS.some(s => attachment[s] !== template[s])
  if (overridden) {
    return {
      label: `CUSTOM (FROM ${template?.name?.toUpperCase() ?? 'TEMPLATE'})`,
      tone: 'amber',
    }
  }
  return {
    label: `FROM ${template?.name?.toUpperCase() ?? 'TEMPLATE'}`,
    tone: 'accent',
  }
}