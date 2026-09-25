package finance

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgreementTemplate is a versioned agreement template.
type AgreementTemplate struct {
	ID          uuid.UUID `json:"id"           db:"id"`
	Name        string    `json:"name"         db:"name"`
	Description string    `json:"description"  db:"description"`
	ContentMD   string    `json:"content_md"   db:"content_md"`
	Version     int       `json:"version"      db:"version"`
	IsActive    bool      `json:"is_active"    db:"is_active"`
	CreatedBy   string    `json:"created_by"   db:"created_by"`
	CreatedAt   time.Time `json:"created_at"   db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"   db:"updated_at"`
}

// AgreementStatus enumerates the lifecycle states.
type AgreementStatus string

const (
	AgreementStatusDraft     AgreementStatus = "draft"
	AgreementStatusSent      AgreementStatus = "sent"
	AgreementStatusSigned    AgreementStatus = "signed"    // at least one party signed
	AgreementStatusCompleted AgreementStatus = "completed" // both parties signed
	AgreementStatusExpired   AgreementStatus = "expired"
	AgreementStatusCancelled AgreementStatus = "cancelled"
)

var validAgreementStatuses = map[AgreementStatus]struct{}{
	AgreementStatusDraft:     {},
	AgreementStatusSent:      {},
	AgreementStatusSigned:    {},
	AgreementStatusCompleted: {},
	AgreementStatusExpired:   {},
	AgreementStatusCancelled: {},
}

func (s AgreementStatus) IsValid() bool {
	_, ok := validAgreementStatuses[s]
	return ok
}

// Scan implements sql.Scanner for AgreementStatus.
func (s *AgreementStatus) Scan(value interface{}) error {
	if value == nil {
		*s = AgreementStatusDraft
		return nil
	}
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot scan %T into AgreementStatus", value)
	}
	*s = AgreementStatus(str)
	return nil
}

// Value implements driver.Valuer for AgreementStatus.
func (s AgreementStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// AgreementInstance is a per-gig agreement instance.
type AgreementInstance struct {
	ID              uuid.UUID       `json:"id"                    db:"id"`
	TemplateID      uuid.UUID       `json:"template_id"           db:"template_id"`
	TemplateVersion int             `json:"template_version"      db:"template_version"`
	GigID           uuid.UUID       `json:"gig_id"                db:"gig_id"`
	ContentMD       string          `json:"content_md"            db:"content_md"`
	DocumentID      *uuid.UUID      `json:"document_id,omitempty" db:"document_id"`
	Status          AgreementStatus `json:"status"                db:"status"`
	RequiredSigners []string        `json:"required_signers"      db:"required_signers"`
	DJSignedAt      *time.Time      `json:"dj_signed_at,omitempty" db:"dj_signed_at"`
	DJSignedBy      string          `json:"dj_signed_by,omitempty" db:"dj_signed_by"`
	ClientSignedAt  *time.Time      `json:"client_signed_at,omitempty" db:"client_signed_at"`
	ClientSignedBy  string          `json:"client_signed_by,omitempty" db:"client_signed_by"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"  db:"expires_at"`
	InternalNotes   string          `json:"internal_notes"        db:"internal_notes"`
	UpdatedAt       time.Time       `json:"updated_at"            db:"updated_at"`
	CreatedAt       time.Time       `json:"created_at"            db:"created_at"`
}

var (
	ErrAgreementTemplateNotFound = errors.New("agreement template not found")
	ErrAgreementInstanceNotFound = errors.New("agreement instance not found")
	ErrAgreementConflict         = errors.New("agreement updated by another writer")
	ErrAgreementValidation       = errors.New("invalid agreement")
	ErrAgreementBadState         = errors.New("invalid agreement state transition")
)

// CreateAgreementTemplateRequest is the body of POST /api/v1/finance/agreements/templates.
type CreateAgreementTemplateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ContentMD   string `json:"content_md"`
	CreatedBy   string `json:"created_by,omitempty"`
}

// UpdateAgreementTemplateRequest is the body of PUT /api/v1/finance/agreements/templates/{id}.
type UpdateAgreementTemplateRequest struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ContentMD   string    `json:"content_md"`
	IsActive    bool      `json:"is_active"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateAgreementInstanceRequest is the body of POST /api/v1/finance/agreements/instances.
type CreateAgreementInstanceRequest struct {
	TemplateID      uuid.UUID  `json:"template_id"`
	GigID           uuid.UUID  `json:"gig_id"`
	RequiredSigners []string   `json:"required_signers,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	InternalNotes   string     `json:"internal_notes,omitempty"`
}

// UpdateAgreementInstanceRequest is the body of PUT /api/v1/finance/agreements/instances/{id}.
type UpdateAgreementInstanceRequest struct {
	Status          AgreementStatus `json:"status,omitempty"`
	RequiredSigners []string        `json:"required_signers,omitempty"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`
	InternalNotes   string          `json:"internal_notes,omitempty"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// SignAgreementRequest is the body of POST /api/v1/finance/agreements/instances/{id}/sign.
type SignAgreementRequest struct {
	SignerRole string    `json:"signer_role"` // "dj" or "client"
	SignedBy   string    `json:"signed_by"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// AgreementTemplateRepositoryIface is the subset the service uses.
type AgreementTemplateRepositoryIface interface {
	Create(ctx context.Context, req CreateAgreementTemplateRequest) (*AgreementTemplate, error)
	GetByID(ctx context.Context, id uuid.UUID) (*AgreementTemplate, error)
	GetLatestByName(ctx context.Context, name string) (*AgreementTemplate, error)
	List(ctx context.Context, activeOnly bool) ([]*AgreementTemplate, error)
	Update(ctx context.Context, id uuid.UUID, req UpdateAgreementTemplateRequest) (*AgreementTemplate, error)
}

// AgreementInstanceRepositoryIface is the subset the service uses.
type AgreementInstanceRepositoryIface interface {
	Create(ctx context.Context, req CreateAgreementInstanceRequest, contentMD string, templateVersion int) (*AgreementInstance, error)
	GetByID(ctx context.Context, id uuid.UUID) (*AgreementInstance, error)
	GetByGigID(ctx context.Context, gigID uuid.UUID) ([]*AgreementInstance, error)
	List(ctx context.Context, status AgreementStatus) ([]*AgreementInstance, error)
	Update(ctx context.Context, id uuid.UUID, req UpdateAgreementInstanceRequest) (*AgreementInstance, error)
	Sign(ctx context.Context, id uuid.UUID, req SignAgreementRequest) (*AgreementInstance, error)
}

// AgreementTemplateRepository persists AgreementTemplate rows.
type AgreementTemplateRepository struct {
	db *pgxpool.Pool
}

func NewAgreementTemplateRepository(db *pgxpool.Pool) *AgreementTemplateRepository {
	return &AgreementTemplateRepository{db: db}
}

var _ AgreementTemplateRepositoryIface = (*AgreementTemplateRepository)(nil)

func (r *AgreementTemplateRepository) Create(ctx context.Context, req CreateAgreementTemplateRequest) (*AgreementTemplate, error) {
	const sql = `
		INSERT INTO agreement_templates (name, description, content_md, version, is_active, created_by)
		VALUES ($1,$2,$3,1,true,$4)
		RETURNING id, name, description, content_md, version, is_active, created_by, created_at, updated_at
	`
	var t AgreementTemplate
	err := r.db.QueryRow(ctx, sql, req.Name, req.Description, req.ContentMD, req.CreatedBy).Scan(
		&t.ID, &t.Name, &t.Description, &t.ContentMD, &t.Version, &t.IsActive, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert template: %w", err)
	}
	return &t, nil
}

func (r *AgreementTemplateRepository) GetByID(ctx context.Context, id uuid.UUID) (*AgreementTemplate, error) {
	const sql = `
		SELECT id, name, description, content_md, version, is_active, created_by, created_at, updated_at
		FROM agreement_templates WHERE id = $1
	`
	var t AgreementTemplate
	err := r.db.QueryRow(ctx, sql, id).Scan(
		&t.ID, &t.Name, &t.Description, &t.ContentMD, &t.Version, &t.IsActive, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgreementTemplateNotFound
		}
		return nil, fmt.Errorf("get template: %w", err)
	}
	return &t, nil
}

func (r *AgreementTemplateRepository) GetLatestByName(ctx context.Context, name string) (*AgreementTemplate, error) {
	const sql = `
		SELECT id, name, description, content_md, version, is_active, created_by, created_at, updated_at
		FROM agreement_templates WHERE name = $1 ORDER BY version DESC LIMIT 1
	`
	var t AgreementTemplate
	err := r.db.QueryRow(ctx, sql, name).Scan(
		&t.ID, &t.Name, &t.Description, &t.ContentMD, &t.Version, &t.IsActive, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgreementTemplateNotFound
		}
		return nil, fmt.Errorf("get latest template: %w", err)
	}
	return &t, nil
}

func (r *AgreementTemplateRepository) List(ctx context.Context, activeOnly bool) ([]*AgreementTemplate, error) {
	sqlStr := `
		SELECT id, name, description, content_md, version, is_active, created_by, created_at, updated_at
		FROM agreement_templates
	`
	if activeOnly {
		sqlStr += " WHERE is_active = true"
	}
	sqlStr += " ORDER BY name, version DESC"
	rows, err := r.db.Query(ctx, sqlStr)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var out []*AgreementTemplate
	for rows.Next() {
		var t AgreementTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.ContentMD, &t.Version, &t.IsActive, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (r *AgreementTemplateRepository) Update(ctx context.Context, id uuid.UUID, req UpdateAgreementTemplateRequest) (*AgreementTemplate, error) {
	const sql = `
		UPDATE agreement_templates
		SET name = $1, description = $2, content_md = $3, is_active = $4, updated_at = now()
		WHERE id = $5 AND updated_at = $6
		RETURNING id, name, description, content_md, version, is_active, created_by, created_at, updated_at
	`
	var t AgreementTemplate
	err := r.db.QueryRow(ctx, sql, req.Name, req.Description, req.ContentMD, req.IsActive, id, req.UpdatedAt).Scan(
		&t.ID, &t.Name, &t.Description, &t.ContentMD, &t.Version, &t.IsActive, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgreementConflict
		}
		return nil, fmt.Errorf("update template: %w", err)
	}
	return &t, nil
}

// AgreementInstanceRepository persists AgreementInstance rows.
type AgreementInstanceRepository struct {
	db *pgxpool.Pool
}

func NewAgreementInstanceRepository(db *pgxpool.Pool) *AgreementInstanceRepository {
	return &AgreementInstanceRepository{db: db}
}

var _ AgreementInstanceRepositoryIface = (*AgreementInstanceRepository)(nil)

func (r *AgreementInstanceRepository) Create(ctx context.Context, req CreateAgreementInstanceRequest, contentMD string, templateVersion int) (*AgreementInstance, error) {
	// Get required_signers as PostgreSQL array
	signers := req.RequiredSigners
	if len(signers) == 0 {
		signers = []string{"dj", "client"}
	}

	const sql = `
		INSERT INTO agreement_instances (template_id, template_version, gig_id, content_md, status, required_signers, expires_at, internal_notes)
		VALUES ($1,$2,$3,$4,'draft',$5,$6,$7)
		RETURNING id, template_id, template_version, gig_id, content_md, document_id, status, required_signers,
			dj_signed_at, dj_signed_by, client_signed_at, client_signed_by, expires_at, internal_notes, updated_at, created_at
	`
	var inst AgreementInstance
	err := r.db.QueryRow(ctx, sql,
		req.TemplateID, templateVersion, req.GigID, contentMD, signers, req.ExpiresAt, req.InternalNotes,
	).Scan(
		&inst.ID, &inst.TemplateID, &inst.TemplateVersion, &inst.GigID, &inst.ContentMD, &inst.DocumentID,
		&inst.Status, &inst.RequiredSigners, &inst.DJSignedAt, &inst.DJSignedBy,
		&inst.ClientSignedAt, &inst.ClientSignedBy, &inst.ExpiresAt, &inst.InternalNotes,
		&inst.UpdatedAt, &inst.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert instance: %w", err)
	}
	return &inst, nil
}

func (r *AgreementInstanceRepository) GetByID(ctx context.Context, id uuid.UUID) (*AgreementInstance, error) {
	const sql = `
		SELECT id, template_id, template_version, gig_id, content_md, document_id, status, required_signers,
			dj_signed_at, dj_signed_by, client_signed_at, client_signed_by, expires_at, internal_notes, updated_at, created_at
		FROM agreement_instances WHERE id = $1
	`
	var inst AgreementInstance
	err := r.db.QueryRow(ctx, sql, id).Scan(
		&inst.ID, &inst.TemplateID, &inst.TemplateVersion, &inst.GigID, &inst.ContentMD, &inst.DocumentID,
		&inst.Status, &inst.RequiredSigners, &inst.DJSignedAt, &inst.DJSignedBy,
		&inst.ClientSignedAt, &inst.ClientSignedBy, &inst.ExpiresAt, &inst.InternalNotes,
		&inst.UpdatedAt, &inst.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgreementInstanceNotFound
		}
		return nil, fmt.Errorf("get instance: %w", err)
	}
	return &inst, nil
}

func (r *AgreementInstanceRepository) GetByGigID(ctx context.Context, gigID uuid.UUID) ([]*AgreementInstance, error) {
	const sql = `
		SELECT id, template_id, template_version, gig_id, content_md, document_id, status, required_signers,
			dj_signed_at, dj_signed_by, client_signed_at, client_signed_by, expires_at, internal_notes, updated_at, created_at
		FROM agreement_instances WHERE gig_id = $1 ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, sql, gigID)
	if err != nil {
		return nil, fmt.Errorf("list instances by gig: %w", err)
	}
	defer rows.Close()

	var out []*AgreementInstance
	for rows.Next() {
		var inst AgreementInstance
		if err := rows.Scan(&inst.ID, &inst.TemplateID, &inst.TemplateVersion, &inst.GigID, &inst.ContentMD, &inst.DocumentID,
			&inst.Status, &inst.RequiredSigners, &inst.DJSignedAt, &inst.DJSignedBy,
			&inst.ClientSignedAt, &inst.ClientSignedBy, &inst.ExpiresAt, &inst.InternalNotes,
			&inst.UpdatedAt, &inst.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		out = append(out, &inst)
	}
	return out, rows.Err()
}

func (r *AgreementInstanceRepository) List(ctx context.Context, status AgreementStatus) ([]*AgreementInstance, error) {
	sqlStr := `
		SELECT id, template_id, template_version, gig_id, content_md, document_id, status, required_signers,
			dj_signed_at, dj_signed_by, client_signed_at, client_signed_by, expires_at, internal_notes, updated_at, created_at
		FROM agreement_instances
	`
	args := []interface{}{}
	if status != "" {
		sqlStr += " WHERE status = $1"
		args = append(args, status)
	}
	sqlStr += " ORDER BY created_at DESC"
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()

	var out []*AgreementInstance
	for rows.Next() {
		var inst AgreementInstance
		if err := rows.Scan(&inst.ID, &inst.TemplateID, &inst.TemplateVersion, &inst.GigID, &inst.ContentMD, &inst.DocumentID,
			&inst.Status, &inst.RequiredSigners, &inst.DJSignedAt, &inst.DJSignedBy,
			&inst.ClientSignedAt, &inst.ClientSignedBy, &inst.ExpiresAt, &inst.InternalNotes,
			&inst.UpdatedAt, &inst.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		out = append(out, &inst)
	}
	return out, rows.Err()
}

func (r *AgreementInstanceRepository) Update(ctx context.Context, id uuid.UUID, req UpdateAgreementInstanceRequest) (*AgreementInstance, error) {
	const sql = `
		UPDATE agreement_instances
		SET status = COALESCE($1, status),
			required_signers = COALESCE($2, required_signers),
			expires_at = COALESCE($3, expires_at),
			internal_notes = COALESCE($4, internal_notes),
			updated_at = now()
		WHERE id = $5 AND updated_at = $6
		RETURNING id, template_id, template_version, gig_id, content_md, document_id, status, required_signers,
			dj_signed_at, dj_signed_by, client_signed_at, client_signed_by, expires_at, internal_notes, updated_at, created_at
	`
	var inst AgreementInstance
	var statusParam *AgreementStatus
	if req.Status != "" {
		statusParam = &req.Status
	}
	var signersParam *[]string
	if req.RequiredSigners != nil {
		signersParam = &req.RequiredSigners
	}
	err := r.db.QueryRow(ctx, sql,
		statusParam, signersParam, req.ExpiresAt, req.InternalNotes, id, req.UpdatedAt,
	).Scan(
		&inst.ID, &inst.TemplateID, &inst.TemplateVersion, &inst.GigID, &inst.ContentMD, &inst.DocumentID,
		&inst.Status, &inst.RequiredSigners, &inst.DJSignedAt, &inst.DJSignedBy,
		&inst.ClientSignedAt, &inst.ClientSignedBy, &inst.ExpiresAt, &inst.InternalNotes,
		&inst.UpdatedAt, &inst.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgreementConflict
		}
		return nil, fmt.Errorf("update instance: %w", err)
	}
	return &inst, nil
}

// signGuardClause is appended to the Sign UPDATE's WHERE clause so a signer
// can never overwrite an already-completed workflow, re-sign the same role
// twice, sign after expiry, or sign a role that wasn't required to.
const signGuardClause = `
	AND status NOT IN ('cancelled', 'expired', 'completed')
	AND (expires_at IS NULL OR expires_at > now())
	AND $5 = ANY(required_signers)
`

func (r *AgreementInstanceRepository) Sign(ctx context.Context, id uuid.UUID, req SignAgreementRequest) (*AgreementInstance, error) {
	now := time.Now().UTC()
	var sql string
	if req.SignerRole == "dj" {
		sql = `
			UPDATE agreement_instances
			SET dj_signed_at = $1, dj_signed_by = $2,
				status = CASE
					WHEN 'client' = ANY(required_signers) AND client_signed_at IS NOT NULL THEN 'completed'
					WHEN 'client' = ANY(required_signers) THEN 'signed'
					ELSE 'completed'
				END,
				updated_at = $1
			WHERE id = $3 AND updated_at = $4
				AND dj_signed_at IS NULL
				` + signGuardClause + `
			RETURNING id, template_id, template_version, gig_id, content_md, document_id, status, required_signers,
				dj_signed_at, dj_signed_by, client_signed_at, client_signed_by, expires_at, internal_notes, updated_at, created_at
		`
	} else if req.SignerRole == "client" {
		sql = `
			UPDATE agreement_instances
			SET client_signed_at = $1, client_signed_by = $2,
				status = CASE
					WHEN 'dj' = ANY(required_signers) AND dj_signed_at IS NOT NULL THEN 'completed'
					WHEN 'dj' = ANY(required_signers) THEN 'signed'
					ELSE 'completed'
				END,
				updated_at = $1
			WHERE id = $3 AND updated_at = $4
				AND client_signed_at IS NULL
				` + signGuardClause + `
			RETURNING id, template_id, template_version, gig_id, content_md, document_id, status, required_signers,
				dj_signed_at, dj_signed_by, client_signed_at, client_signed_by, expires_at, internal_notes, updated_at, created_at
		`
	} else {
		return nil, fmt.Errorf("%w: signer_role must be 'dj' or 'client'", ErrAgreementValidation)
	}

	var inst AgreementInstance
	err := r.db.QueryRow(ctx, sql, now, req.SignedBy, id, req.UpdatedAt, req.SignerRole).Scan(
		&inst.ID, &inst.TemplateID, &inst.TemplateVersion, &inst.GigID, &inst.ContentMD, &inst.DocumentID,
		&inst.Status, &inst.RequiredSigners, &inst.DJSignedAt, &inst.DJSignedBy,
		&inst.ClientSignedAt, &inst.ClientSignedBy, &inst.ExpiresAt, &inst.InternalNotes,
		&inst.UpdatedAt, &inst.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, r.classifySignFailure(ctx, id, req)
		}
		return nil, fmt.Errorf("sign instance: %w", err)
	}
	return &inst, nil
}

// classifySignFailure re-reads the instance after a Sign UPDATE matched zero
// rows to distinguish *why*: the row doesn't exist, the optimistic-concurrency
// token was stale, or the guard clause rejected the state transition (bad
// state, e.g. cancelled/expired/completed, already signed by that role, or
// role not in required_signers).
func (r *AgreementInstanceRepository) classifySignFailure(ctx context.Context, id uuid.UUID, req SignAgreementRequest) error {
	current, err := r.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrAgreementInstanceNotFound) {
			return ErrAgreementInstanceNotFound
		}
		return fmt.Errorf("classify sign failure: %w", err)
	}
	if !current.UpdatedAt.Equal(req.UpdatedAt) {
		return ErrAgreementConflict
	}
	return ErrAgreementBadState
}

// AgreementTemplateService coordinates template operations.
type AgreementTemplateService struct {
	repo AgreementTemplateRepositoryIface
}

func NewAgreementTemplateService(repo AgreementTemplateRepositoryIface) *AgreementTemplateService {
	return &AgreementTemplateService{repo: repo}
}

func (s *AgreementTemplateService) Create(ctx context.Context, req CreateAgreementTemplateRequest) (*AgreementTemplate, error) {
	if err := ValidateAgreementTemplateRequest(req); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, req)
}

func (s *AgreementTemplateService) GetByID(ctx context.Context, id uuid.UUID) (*AgreementTemplate, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *AgreementTemplateService) GetLatestByName(ctx context.Context, name string) (*AgreementTemplate, error) {
	return s.repo.GetLatestByName(ctx, name)
}

func (s *AgreementTemplateService) List(ctx context.Context, activeOnly bool) ([]*AgreementTemplate, error) {
	return s.repo.List(ctx, activeOnly)
}

func (s *AgreementTemplateService) Update(ctx context.Context, id uuid.UUID, req UpdateAgreementTemplateRequest) (*AgreementTemplate, error) {
	if err := ValidateAgreementTemplateUpdateRequest(req); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, id, req)
}

// AgreementInstanceService coordinates instance operations.
type AgreementInstanceService struct {
	repo         AgreementInstanceRepositoryIface
	templateRepo AgreementTemplateRepositoryIface
	docService   DocumentServiceIface // for PDF generation
}

// DocumentServiceIface is the subset needed for agreement PDF.
type DocumentServiceIface interface {
	Create(ctx context.Context, req CreateDocumentRequest, content io.Reader) (*Document, error)
}

func NewAgreementInstanceService(repo AgreementInstanceRepositoryIface, templateRepo AgreementTemplateRepositoryIface, docService DocumentServiceIface) *AgreementInstanceService {
	return &AgreementInstanceService{repo: repo, templateRepo: templateRepo, docService: docService}
}

// RenderContent fills template placeholders with gig/billing data.
func (s *AgreementInstanceService) RenderContent(templateMD string, gigData map[string]string, billingData map[string]string) string {
	content := templateMD
	for k, v := range gigData {
		content = strings.ReplaceAll(content, "{{gig."+k+"}}", v)
	}
	for k, v := range billingData {
		content = strings.ReplaceAll(content, "{{billing."+k+"}}", v)
	}
	return content
}

func (s *AgreementInstanceService) Create(ctx context.Context, req CreateAgreementInstanceRequest) (*AgreementInstance, error) {
	if err := ValidateAgreementInstanceRequest(req); err != nil {
		return nil, err
	}
	// Get template
	template, err := s.templateRepo.GetByID(ctx, req.TemplateID)
	if err != nil {
		return nil, err
	}
	// Render content with placeholder data (simplified for now)
	contentMD := template.ContentMD
	inst, err := s.repo.Create(ctx, req, contentMD, template.Version)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

func (s *AgreementInstanceService) GetByID(ctx context.Context, id uuid.UUID) (*AgreementInstance, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *AgreementInstanceService) GetByGigID(ctx context.Context, gigID uuid.UUID) ([]*AgreementInstance, error) {
	return s.repo.GetByGigID(ctx, gigID)
}

func (s *AgreementInstanceService) List(ctx context.Context, status AgreementStatus) ([]*AgreementInstance, error) {
	return s.repo.List(ctx, status)
}

func (s *AgreementInstanceService) Update(ctx context.Context, id uuid.UUID, req UpdateAgreementInstanceRequest) (*AgreementInstance, error) {
	if req.Status != "" && !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status", ErrAgreementValidation)
	}
	return s.repo.Update(ctx, id, req)
}

func (s *AgreementInstanceService) Sign(ctx context.Context, id uuid.UUID, req SignAgreementRequest) (*AgreementInstance, error) {
	if req.SignerRole != "dj" && req.SignerRole != "client" {
		return nil, fmt.Errorf("%w: signer_role must be 'dj' or 'client'", ErrAgreementValidation)
	}
	if req.SignedBy == "" {
		return nil, fmt.Errorf("%w: signed_by is required", ErrAgreementValidation)
	}
	if req.UpdatedAt.IsZero() {
		return nil, fmt.Errorf("%w: updated_at is required", ErrAgreementValidation)
	}
	return s.repo.Sign(ctx, id, req)
}

func (s *AgreementInstanceService) GeneratePDF(ctx context.Context, id uuid.UUID) (*Document, error) {
	inst, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// For now, just create a document with the markdown content as text
	// In production, this would render to PDF via a renderer
	content := []byte("# " + inst.TemplateID.String() + "\n\n" + inst.ContentMD)
	req := CreateDocumentRequest{
		OwnerType:  DocumentOwnerAgreement,
		OwnerID:    inst.ID,
		Filename:   fmt.Sprintf("agreement-%s.md", inst.ID.String()[:8]),
		MimeType:   "text/markdown",
		UploadedBy: "system",
	}
	return s.docService.Create(ctx, req, bytes.NewReader(content))
}

// Validation

type AgreementTemplateValidationErrors []AgreementFieldError

func (es AgreementTemplateValidationErrors) Error() string {
	if len(es) == 0 {
		return "validation failed"
	}
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

func (es AgreementTemplateValidationErrors) Is(target error) bool {
	return target == ErrAgreementValidation
}
func (es AgreementTemplateValidationErrors) Unwrap() error { return ErrAgreementValidation }

type AgreementInstanceValidationErrors []AgreementFieldError

func (es AgreementInstanceValidationErrors) Error() string {
	if len(es) == 0 {
		return "validation failed"
	}
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

func (es AgreementInstanceValidationErrors) Is(target error) bool {
	return target == ErrAgreementValidation
}
func (es AgreementInstanceValidationErrors) Unwrap() error { return ErrAgreementValidation }

type AgreementFieldError struct {
	Field   string
	Message string
}

func (e AgreementFieldError) Error() string { return e.Field + ": " + e.Message }

func ValidateAgreementTemplateRequest(req CreateAgreementTemplateRequest) error {
	if req.Name == "" {
		return AgreementTemplateValidationErrors{{Field: "name", Message: "required"}}
	}
	if req.ContentMD == "" {
		return AgreementTemplateValidationErrors{{Field: "content_md", Message: "required"}}
	}
	return nil
}

func ValidateAgreementTemplateUpdateRequest(req UpdateAgreementTemplateRequest) error {
	if req.Name == "" {
		return AgreementTemplateValidationErrors{{Field: "name", Message: "required"}}
	}
	if req.ContentMD == "" {
		return AgreementTemplateValidationErrors{{Field: "content_md", Message: "required"}}
	}
	return nil
}

func ValidateAgreementInstanceRequest(req CreateAgreementInstanceRequest) error {
	var errs AgreementInstanceValidationErrors
	if req.TemplateID == uuid.Nil {
		errs = append(errs, AgreementFieldError{Field: "template_id", Message: "required"})
	}
	if req.GigID == uuid.Nil {
		errs = append(errs, AgreementFieldError{Field: "gig_id", Message: "required"})
	}
	for _, s := range req.RequiredSigners {
		if s != "dj" && s != "client" {
			errs = append(errs, AgreementFieldError{Field: "required_signers", Message: "must be 'dj' or 'client'"})
			break
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

var _ = uuid.Nil
