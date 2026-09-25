package finance

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeTemplateRepo is an in-memory AgreementTemplateRepository.
type fakeTemplateRepo struct {
	templates map[uuid.UUID]*AgreementTemplate
}

func newFakeTemplateRepo() *fakeTemplateRepo {
	return &fakeTemplateRepo{templates: make(map[uuid.UUID]*AgreementTemplate)}
}

func (f *fakeTemplateRepo) Create(ctx context.Context, req CreateAgreementTemplateRequest) (*AgreementTemplate, error) {
	t := &AgreementTemplate{
		ID:          uuid.New(),
		Name:        req.Name,
		Description: req.Description,
		ContentMD:   req.ContentMD,
		Version:     len(f.templates) + 1,
		IsActive:    true,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	f.templates[t.ID] = t
	return t, nil
}

func (f *fakeTemplateRepo) GetByID(ctx context.Context, id uuid.UUID) (*AgreementTemplate, error) {
	t, ok := f.templates[id]
	if !ok {
		return nil, ErrAgreementTemplateNotFound
	}
	cp := *t
	return &cp, nil
}

func (f *fakeTemplateRepo) GetLatestByName(ctx context.Context, name string) (*AgreementTemplate, error) {
	var latest *AgreementTemplate
	for _, t := range f.templates {
		if t.Name == name {
			if latest == nil || t.Version > latest.Version {
				latest = t
			}
		}
	}
	if latest == nil {
		return nil, ErrAgreementTemplateNotFound
	}
	cp := *latest
	return &cp, nil
}

func (f *fakeTemplateRepo) List(ctx context.Context, activeOnly bool) ([]*AgreementTemplate, error) {
	var out []*AgreementTemplate
	for _, t := range f.templates {
		if activeOnly && !t.IsActive {
			continue
		}
		cp := *t
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeTemplateRepo) Update(ctx context.Context, id uuid.UUID, req UpdateAgreementTemplateRequest) (*AgreementTemplate, error) {
	t, ok := f.templates[id]
	if !ok || !req.UpdatedAt.Equal(t.UpdatedAt) {
		return nil, ErrAgreementConflict
	}
	t.Name = req.Name
	t.Description = req.Description
	t.ContentMD = req.ContentMD
	t.IsActive = req.IsActive
	t.UpdatedAt = time.Now().UTC()
	return t, nil
}

// fakeInstanceRepo is an in-memory AgreementInstanceRepository.
type fakeInstanceRepo struct {
	instances map[uuid.UUID]*AgreementInstance
}

func newFakeInstanceRepo() *fakeInstanceRepo {
	return &fakeInstanceRepo{instances: make(map[uuid.UUID]*AgreementInstance)}
}

func (f *fakeInstanceRepo) Create(ctx context.Context, req CreateAgreementInstanceRequest, contentMD string, templateVersion int) (*AgreementInstance, error) {
	signers := req.RequiredSigners
	if len(signers) == 0 {
		signers = []string{"dj", "client"}
	}
	inst := &AgreementInstance{
		ID:              uuid.New(),
		TemplateID:      req.TemplateID,
		TemplateVersion: templateVersion,
		GigID:           req.GigID,
		ContentMD:       contentMD,
		Status:          AgreementStatusDraft,
		RequiredSigners: signers,
		ExpiresAt:       req.ExpiresAt,
		InternalNotes:   req.InternalNotes,
		UpdatedAt:       time.Now().UTC(),
		CreatedAt:       time.Now().UTC(),
	}
	f.instances[inst.ID] = inst
	return inst, nil
}

func (f *fakeInstanceRepo) GetByID(ctx context.Context, id uuid.UUID) (*AgreementInstance, error) {
	inst, ok := f.instances[id]
	if !ok {
		return nil, ErrAgreementInstanceNotFound
	}
	cp := *inst
	return &cp, nil
}

func (f *fakeInstanceRepo) GetByGigID(ctx context.Context, gigID uuid.UUID) ([]*AgreementInstance, error) {
	var out []*AgreementInstance
	for _, inst := range f.instances {
		if inst.GigID == gigID {
			cp := *inst
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeInstanceRepo) List(ctx context.Context, status AgreementStatus) ([]*AgreementInstance, error) {
	var out []*AgreementInstance
	for _, inst := range f.instances {
		if status != "" && inst.Status != status {
			continue
		}
		cp := *inst
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeInstanceRepo) Update(ctx context.Context, id uuid.UUID, req UpdateAgreementInstanceRequest) (*AgreementInstance, error) {
	inst, ok := f.instances[id]
	if !ok || !req.UpdatedAt.Equal(inst.UpdatedAt) {
		return nil, ErrAgreementConflict
	}
	if req.Status != "" {
		inst.Status = req.Status
	}
	if req.RequiredSigners != nil {
		inst.RequiredSigners = req.RequiredSigners
	}
	if req.ExpiresAt != nil {
		inst.ExpiresAt = req.ExpiresAt
	}
	if req.InternalNotes != "" {
		inst.InternalNotes = req.InternalNotes
	}
	inst.UpdatedAt = time.Now().UTC()
	return inst, nil
}

func (f *fakeInstanceRepo) Sign(ctx context.Context, id uuid.UUID, req SignAgreementRequest) (*AgreementInstance, error) {
	inst, ok := f.instances[id]
	if !ok || !req.UpdatedAt.Equal(inst.UpdatedAt) {
		return nil, ErrAgreementConflict
	}
	now := time.Now().UTC()
	if req.SignerRole == "dj" {
		inst.DJSignedAt = &now
		inst.DJSignedBy = req.SignedBy
	} else if req.SignerRole == "client" {
		inst.ClientSignedAt = &now
		inst.ClientSignedBy = req.SignedBy
	}
	// Simulate status transition
	djSigned := inst.DJSignedAt != nil
	clientSigned := inst.ClientSignedAt != nil
	if djSigned && clientSigned {
		inst.Status = AgreementStatusCompleted
	} else if djSigned || clientSigned {
		inst.Status = AgreementStatusSigned
	}
	inst.UpdatedAt = now
	return inst, nil
}

func TestAgreementTemplateService_CreateAndGet(t *testing.T) {
	repo := newFakeTemplateRepo()
	svc := NewAgreementTemplateService(repo)

	req := CreateAgreementTemplateRequest{
		Name:        "Standard Performance Agreement",
		Description: "Default agreement for all gigs",
		ContentMD:   "# Agreement\n\nThis agreement is between {{billing.legal_name}} and {{gig.client_name}}...",
		CreatedBy:   "system",
	}

	tpl, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tpl.ID == uuid.Nil {
		t.Fatal("missing id")
	}
	if tpl.Version != 1 {
		t.Errorf("version: %d", tpl.Version)
	}

	// Get
	got, err := svc.GetByID(context.Background(), tpl.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != req.Name {
		t.Errorf("name mismatch: %s", got.Name)
	}
}

func TestAgreementTemplateService_RejectsEmptyFields(t *testing.T) {
	repo := newFakeTemplateRepo()
	svc := NewAgreementTemplateService(repo)

	req := CreateAgreementTemplateRequest{Name: "", ContentMD: ""}
	_, err := svc.Create(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestAgreementTemplateService_UpdateWithConcurrencyToken(t *testing.T) {
	repo := newFakeTemplateRepo()
	svc := NewAgreementTemplateService(repo)

	tpl, _ := svc.Create(context.Background(), CreateAgreementTemplateRequest{
		Name:      "Test",
		ContentMD: "# v1",
	})

	// Update with stale token
	stale := tpl.UpdatedAt.Add(-time.Hour)
	_, err := svc.Update(context.Background(), tpl.ID, UpdateAgreementTemplateRequest{
		Name:      "Test",
		ContentMD: "# v2",
		IsActive:  true,
		UpdatedAt: stale,
	})
	if err != ErrAgreementConflict {
		t.Errorf("expected conflict, got: %v", err)
	}

	// Update with correct token
	_, err = svc.Update(context.Background(), tpl.ID, UpdateAgreementTemplateRequest{
		Name:      "Test",
		ContentMD: "# v2",
		IsActive:  true,
		UpdatedAt: tpl.UpdatedAt,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAgreementInstanceService_CreateAndSignLifecycle(t *testing.T) {
	tplRepo := newFakeTemplateRepo()
	instRepo := newFakeInstanceRepo()
	tplSvc := NewAgreementTemplateService(tplRepo)
	instSvc := NewAgreementInstanceService(instRepo, tplRepo, nil)

	// Create template
	tpl, _ := tplSvc.Create(context.Background(), CreateAgreementTemplateRequest{
		Name:      "Standard",
		ContentMD: "## Agreement\n\nFor: {{gig.client_name}}",
	})

	// Create instance
	gigID := uuid.New()
	req := CreateAgreementInstanceRequest{
		TemplateID:      tpl.ID,
		GigID:           gigID,
		RequiredSigners: []string{"dj", "client"},
	}
	inst, err := instSvc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create instance: %v", err)
	}
	if inst.Status != AgreementStatusDraft {
		t.Errorf("initial status: %s", inst.Status)
	}

	// DJ signs
	token := inst.UpdatedAt
	djSigned, err := instSvc.Sign(context.Background(), inst.ID, SignAgreementRequest{
		SignerRole: "dj",
		SignedBy:   "user-123",
		UpdatedAt:  token,
	})
	if err != nil {
		t.Fatalf("DJ sign: %v", err)
	}
	if djSigned.DJSignedAt == nil {
		t.Error("DJ signed_at not set")
	}
	if djSigned.Status != AgreementStatusSigned {
		t.Errorf("after dj sign, status: %s", djSigned.Status)
	}

	// Client signs
	clientSigned, err := instSvc.Sign(context.Background(), inst.ID, SignAgreementRequest{
		SignerRole: "client",
		SignedBy:   "client-456",
		UpdatedAt:  djSigned.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("Client sign: %v", err)
	}
	if clientSigned.ClientSignedAt == nil {
		t.Error("Client signed_at not set")
	}
	if clientSigned.Status != AgreementStatusCompleted {
		t.Errorf("after both sign, status: %s", clientSigned.Status)
	}
}

func TestAgreementInstanceService_ValidateSigners(t *testing.T) {
	instSvc := NewAgreementInstanceService(nil, nil, nil)

	// Invalid signer role
	_, err := instSvc.Sign(context.Background(), uuid.New(), SignAgreementRequest{
		SignerRole: "alien",
		SignedBy:   "x",
		UpdatedAt:  time.Now(),
	})
	if err == nil {
		t.Error("expected error for invalid signer_role")
	}

	// Missing signed_by
	_, err = instSvc.Sign(context.Background(), uuid.New(), SignAgreementRequest{
		SignerRole: "dj",
		SignedBy:   "",
		UpdatedAt:  time.Now(),
	})
	if err == nil {
		t.Error("expected error for missing signed_by")
	}
}

func TestAgreementInstanceService_Validation(t *testing.T) {
	instSvc := NewAgreementInstanceService(nil, nil, nil)

	// Missing template_id
	req := CreateAgreementInstanceRequest{GigID: uuid.New()}
	_, err := instSvc.Create(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error")
	}

	// Missing gig_id
	req = CreateAgreementInstanceRequest{TemplateID: uuid.New()}
	_, err = instSvc.Create(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error")
	}

	// Invalid signer
	req = CreateAgreementInstanceRequest{
		TemplateID:      uuid.New(),
		GigID:           uuid.New(),
		RequiredSigners: []string{"alien"},
	}
	_, err = instSvc.Create(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error for invalid signer")
	}
}

func TestRenderContent_ReplacesPlaceholders(t *testing.T) {
	instSvc := NewAgreementInstanceService(nil, nil, nil)
	gigData := map[string]string{"client_name": "Acme Corp"}
	billingData := map[string]string{"legal_name": "DJ Spin"}

	template := "Agreement between {{billing.legal_name}} and {{gig.client_name}}."
	rendered := instSvc.RenderContent(template, gigData, billingData)

	expected := "Agreement between DJ Spin and Acme Corp."
	if rendered != expected {
		t.Errorf("rendered = %q, want %q", rendered, expected)
	}
}

func TestAgreementInstanceService_GetByGigID(t *testing.T) {
	instRepo := newFakeInstanceRepo()
	instSvc := NewAgreementInstanceService(instRepo, nil, nil)

	gigID := uuid.New()
	for i := 0; i < 3; i++ {
		instRepo.Create(context.Background(), CreateAgreementInstanceRequest{
			TemplateID: uuid.New(),
			GigID:      gigID,
		}, fmt.Sprintf("content %d", i), 1)
	}

	instances, err := instSvc.GetByGigID(context.Background(), gigID)
	if err != nil {
		t.Fatalf("GetByGigID: %v", err)
	}
	if len(instances) != 3 {
		t.Errorf("expected 3, got %d", len(instances))
	}
}

func TestAgreementStatus_IsValid(t *testing.T) {
	tests := []struct {
		status AgreementStatus
		valid  bool
	}{
		{AgreementStatusDraft, true},
		{AgreementStatusSent, true},
		{AgreementStatusSigned, true},
		{AgreementStatusCompleted, true},
		{AgreementStatusExpired, true},
		{AgreementStatusCancelled, true},
		{AgreementStatus("invalid"), false},
	}
	for _, tt := range tests {
		if got := tt.status.IsValid(); got != tt.valid {
			t.Errorf("%s: got %v, want %v", tt.status, got, tt.valid)
		}
	}
}

func TestDocumentService_CreateForAgreement(t *testing.T) {
	// Verify the document service can accept agreement-owned documents
	repo := newFakeDocumentRepo()
	store := newFakeObjectStore()
	svc := NewDocumentService(repo, store, "test-bucket")

	content := []byte("# Agreement\n\nTerms and conditions...")
	req := CreateDocumentRequest{
		OwnerType:  DocumentOwnerAgreement,
		OwnerID:    uuid.New(),
		Filename:   "agreement-123.md",
		MimeType:   "text/markdown",
		UploadedBy: "agreement-service",
	}

	doc, err := svc.Create(context.Background(), req, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if doc.OwnerType != DocumentOwnerAgreement {
		t.Errorf("owner_type: %s", doc.OwnerType)
	}
}