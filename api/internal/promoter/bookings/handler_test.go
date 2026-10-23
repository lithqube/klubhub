package bookings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/klubhub/dj/api/internal/platform/authz"
)

// fakeService implements ServiceIface with canned responses, error injection
// and input capture.
type fakeService struct {
	contacts     []*TalentContact
	slots        []*OpenSlot
	matches      []*TalentMatch
	templates    []*Template
	offers       []*OfferRequest
	suppressions []*Suppression

	err error

	gotFilter   TalentFilter
	gotContact  *TalentContactInput
	gotSlot     *OpenSlotInput
	gotTpl      *TemplateCreate
	gotTplUpd   *TemplateUpdate
	gotSend     *SendOfferInput
	gotSendP    authz.Principal
	gotSupp     *SuppressionCreate
	gotIDs      []uuid.UUID
	gotEventID  uuid.UUID
}

func (f *fakeService) ListTalentContacts(ctx context.Context, filter TalentFilter) ([]*TalentContact, error) {
	f.gotFilter = filter
	return f.contacts, f.err
}

func (f *fakeService) CreateTalentContact(ctx context.Context, in TalentContactInput) (*TalentContact, error) {
	f.gotContact = &in
	if f.err != nil {
		return nil, f.err
	}
	return &TalentContact{ID: uuid.New(), Name: in.Name, Email: in.Email, HasEmail: in.Email != ""}, nil
}

func (f *fakeService) UpdateTalentContact(ctx context.Context, id uuid.UUID, in TalentContactInput) (*TalentContact, error) {
	f.gotIDs = append(f.gotIDs, id)
	f.gotContact = &in
	if f.err != nil {
		return nil, f.err
	}
	return &TalentContact{ID: id, Name: in.Name}, nil
}

func (f *fakeService) DeleteTalentContact(ctx context.Context, id uuid.UUID) error {
	f.gotIDs = append(f.gotIDs, id)
	return f.err
}

func (f *fakeService) SuppressTalentContact(ctx context.Context, id uuid.UUID) (*Suppression, error) {
	f.gotIDs = append(f.gotIDs, id)
	if f.err != nil {
		return nil, f.err
	}
	return &Suppression{ID: uuid.New(), Reason: "marked do-not-contact"}, nil
}

func (f *fakeService) ListOpenSlots(ctx context.Context, eventID uuid.UUID) ([]*OpenSlot, error) {
	f.gotEventID = eventID
	return f.slots, f.err
}

func (f *fakeService) CreateOpenSlot(ctx context.Context, eventID uuid.UUID, in OpenSlotInput) (*OpenSlot, error) {
	f.gotEventID = eventID
	f.gotSlot = &in
	if f.err != nil {
		return nil, f.err
	}
	return &OpenSlot{ID: uuid.New(), EventID: eventID, Role: in.Role}, nil
}

func (f *fakeService) DeleteOpenSlot(ctx context.Context, slotID uuid.UUID) error {
	f.gotIDs = append(f.gotIDs, slotID)
	return f.err
}

func (f *fakeService) MatchSlot(ctx context.Context, slotID uuid.UUID) ([]*TalentMatch, error) {
	f.gotIDs = append(f.gotIDs, slotID)
	return f.matches, f.err
}

func (f *fakeService) ListTemplates(ctx context.Context) ([]*Template, error) { return f.templates, f.err }

func (f *fakeService) CreateTemplate(ctx context.Context, in TemplateCreate) (*Template, error) {
	f.gotTpl = &in
	if f.err != nil {
		return nil, f.err
	}
	return &Template{ID: uuid.New(), Name: in.Name, Version: 1}, nil
}

func (f *fakeService) UpdateTemplate(ctx context.Context, id uuid.UUID, in TemplateUpdate) (*Template, error) {
	f.gotIDs = append(f.gotIDs, id)
	f.gotTplUpd = &in
	if f.err != nil {
		return nil, f.err
	}
	return &Template{ID: id, Version: in.Version + 1}, nil
}

func (f *fakeService) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	f.gotIDs = append(f.gotIDs, id)
	return f.err
}

func (f *fakeService) SendOffer(ctx context.Context, p authz.Principal, in *SendOfferInput) (*OfferRequest, error) {
	f.gotSendP = p
	f.gotSend = in
	if f.err != nil {
		return nil, f.err
	}
	return &OfferRequest{ID: uuid.New(), TalentContactID: &in.TalentContactID, Status: "sent"}, nil
}

func (f *fakeService) ListOffers(ctx context.Context) ([]*OfferRequest, error) { return f.offers, f.err }

func (f *fakeService) GetOffer(ctx context.Context, id uuid.UUID) (*OfferRequest, error) {
	f.gotIDs = append(f.gotIDs, id)
	if f.err != nil {
		return nil, f.err
	}
	return &OfferRequest{ID: id, Status: "sent"}, nil
}

func (f *fakeService) ListSuppressions(ctx context.Context) ([]*Suppression, error) {
	return f.suppressions, f.err
}

func (f *fakeService) AddSuppression(ctx context.Context, in *SuppressionCreate) (*Suppression, error) {
	f.gotSupp = in
	if f.err != nil {
		return nil, f.err
	}
	return &Suppression{ID: uuid.New(), Reason: in.Reason}, nil
}

func (f *fakeService) DeleteSuppression(ctx context.Context, id uuid.UUID) error {
	f.gotIDs = append(f.gotIDs, id)
	return f.err
}

// ─── Mount helpers ───────────────────────────────────────────────────────────

var testOrg = uuid.Must(uuid.NewV7())

func ownerPrincipal() authz.Principal {
	return authz.Principal{
		Sub: "local:" + uuid.NewString(), OrgID: testOrg.String(),
		Roles: []string{"owner"}, AMR: []string{"pwd", "otp"}, AuthTime: time.Now(),
	}
}

// mount builds the handler behind the real policy engine, with a middleware
// that injects the principal (the auth middleware's job in production).
func mount(t *testing.T, svc ServiceIface, p *authz.Principal) (http.Handler, *authz.Registry) {
	t.Helper()
	engine, err := authz.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reg := authz.NewRegistry()
	mux := chi.NewRouter()
	mux.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p != nil {
				r = r.WithContext(authz.ContextWithPrincipal(r.Context(), *p))
			}
			next.ServeHTTP(w, r)
		})
	})
	NewHandler(svc).Mount(mux, engine, reg, nil)
	return mux, reg
}

func do(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, status, rec.Body.String())
	}
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	wantStatus(t, rec, status)
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if body["error"] != code {
		t.Fatalf("error = %q, want %q (body %q)", body["error"], code, rec.Body.String())
	}
}

// ─── Route/action contract ───────────────────────────────────────────────────

func TestRouteActions(t *testing.T) {
	_, reg := mount(t, &fakeService{}, ptrPrincipal(ownerPrincipal()))
	want := []struct {
		method, pattern, action string
	}{
		{http.MethodGet, "/api/v1/bookings/talent", "talent.read"},
		{http.MethodPost, "/api/v1/bookings/talent", "talent.write"},
		{http.MethodPut, "/api/v1/bookings/talent/{contactID}", "talent.write"},
		{http.MethodDelete, "/api/v1/bookings/talent/{contactID}", "talent.write"},
		{http.MethodPost, "/api/v1/bookings/talent/{contactID}/suppress", "talent.write"},
		{http.MethodGet, "/api/v1/bookings/events/{eventID}/slots", "booking.read"},
		{http.MethodPost, "/api/v1/bookings/events/{eventID}/slots", "booking.write"},
		{http.MethodDelete, "/api/v1/bookings/slots/{slotID}", "booking.write"},
		{http.MethodGet, "/api/v1/bookings/slots/{slotID}/matches", "talent.read"},
		{http.MethodGet, "/api/v1/bookings/templates", "booking.read"},
		{http.MethodPost, "/api/v1/bookings/templates", "booking.write"},
		{http.MethodPut, "/api/v1/bookings/templates/{templateID}", "booking.write"},
		{http.MethodDelete, "/api/v1/bookings/templates/{templateID}", "booking.write"},
		{http.MethodPost, "/api/v1/bookings/offers", "talent.write"},
		{http.MethodGet, "/api/v1/bookings/offers", "talent.read"},
		{http.MethodGet, "/api/v1/bookings/offers/{offerID}", "talent.read"},
		{http.MethodGet, "/api/v1/bookings/suppressions", "talent.read"},
		{http.MethodPost, "/api/v1/bookings/suppressions", "talent.write"},
		{http.MethodDelete, "/api/v1/bookings/suppressions/{suppressionID}", "talent.write"},
	}
	for _, w := range want {
		rt, ok := reg.Lookup(w.method, w.pattern)
		if !ok {
			t.Errorf("%s %s: not registered", w.method, w.pattern)
			continue
		}
		if rt.Public {
			t.Errorf("%s %s: must not be public", w.method, w.pattern)
		}
		if rt.Action != w.action {
			t.Errorf("%s %s: action = %q, want %q", w.method, w.pattern, rt.Action, w.action)
		}
	}
}

func ptrPrincipal(p authz.Principal) *authz.Principal { return &p }

// ─── Happy paths ─────────────────────────────────────────────────────────────

func TestHandlerTalentCRUD(t *testing.T) {
	svc := &fakeService{contacts: []*TalentContact{{ID: uuid.New(), Name: "Nadia", HasEmail: true}}}
	h, _ := mount(t, svc, ptrPrincipal(ownerPrincipal()))

	rec := do(t, h, http.MethodGet, "/api/v1/bookings/talent?city=Berlin&genre=techno&q=nadia", "")
	wantStatus(t, rec, http.StatusOK)
	if svc.gotFilter.City != "Berlin" || svc.gotFilter.Genre != "techno" || svc.gotFilter.Q != "nadia" {
		t.Fatalf("filter not passed through: %+v", svc.gotFilter)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/bookings/talent", `{"name":"Nadia Voss","email":"nadia@example.com","genres":["techno"]}`)
	wantStatus(t, rec, http.StatusCreated)
	if svc.gotContact == nil || svc.gotContact.Name != "Nadia Voss" {
		t.Fatalf("create body not decoded: %+v", svc.gotContact)
	}

	id := uuid.NewString()
	rec = do(t, h, http.MethodPut, "/api/v1/bookings/talent/"+id, `{"name":"Nadia V."}`)
	wantStatus(t, rec, http.StatusOK)
	if len(svc.gotIDs) != 1 || svc.gotIDs[0].String() != id {
		t.Fatalf("contactID not parsed: %v", svc.gotIDs)
	}

	rec = do(t, h, http.MethodDelete, "/api/v1/bookings/talent/"+id, "")
	wantStatus(t, rec, http.StatusNoContent)

	rec = do(t, h, http.MethodPost, "/api/v1/bookings/talent/"+id+"/suppress", "")
	wantStatus(t, rec, http.StatusCreated)
}

func TestHandlerSlotsMatchesTemplates(t *testing.T) {
	svc := &fakeService{
		slots:     []*OpenSlot{{ID: uuid.New(), Role: "Headliner"}},
		matches:   []*TalentMatch{{Contact: TalentContact{ID: uuid.New(), Name: "Nadia"}}},
		templates: []*Template{{ID: uuid.New(), Name: "Default"}},
	}
	h, _ := mount(t, svc, ptrPrincipal(ownerPrincipal()))

	eventID := uuid.NewString()
	rec := do(t, h, http.MethodGet, "/api/v1/bookings/events/"+eventID+"/slots", "")
	wantStatus(t, rec, http.StatusOK)
	if svc.gotEventID.String() != eventID {
		t.Fatalf("eventID not parsed: %v", svc.gotEventID)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/bookings/events/"+eventID+"/slots", `{"role":"Headliner","fee_min":500}`)
	wantStatus(t, rec, http.StatusCreated)
	if svc.gotSlot == nil || svc.gotSlot.Role != "Headliner" {
		t.Fatalf("slot body not decoded: %+v", svc.gotSlot)
	}

	slotID := uuid.NewString()
	rec = do(t, h, http.MethodDelete, "/api/v1/bookings/slots/"+slotID, "")
	wantStatus(t, rec, http.StatusNoContent)

	rec = do(t, h, http.MethodGet, "/api/v1/bookings/slots/"+slotID+"/matches", "")
	wantStatus(t, rec, http.StatusOK)

	rec = do(t, h, http.MethodGet, "/api/v1/bookings/templates", "")
	wantStatus(t, rec, http.StatusOK)
	rec = do(t, h, http.MethodPost, "/api/v1/bookings/templates", `{"name":"A","subject":"s","body":"b"}`)
	wantStatus(t, rec, http.StatusCreated)
	tplID := uuid.NewString()
	rec = do(t, h, http.MethodPut, "/api/v1/bookings/templates/"+tplID, `{"version":1,"subject":"s2"}`)
	wantStatus(t, rec, http.StatusOK)
	if svc.gotTplUpd == nil || svc.gotTplUpd.Version != 1 || svc.gotTplUpd.Subject == nil {
		t.Fatalf("template update not decoded: %+v", svc.gotTplUpd)
	}
	rec = do(t, h, http.MethodDelete, "/api/v1/bookings/templates/"+tplID, "")
	wantStatus(t, rec, http.StatusNoContent)
}

func TestHandlerOffersAndSuppressions(t *testing.T) {
	svc := &fakeService{
		offers:       []*OfferRequest{{ID: uuid.New(), Status: "sent"}},
		suppressions: []*Suppression{{ID: uuid.New(), Reason: "opt-out"}},
	}
	p := ownerPrincipal()
	h, _ := mount(t, svc, ptrPrincipal(p))

	contactID := uuid.NewString()
	rec := do(t, h, http.MethodPost, "/api/v1/bookings/offers", `{"talent_contact_id":"`+contactID+`"}`)
	wantStatus(t, rec, http.StatusCreated)
	if svc.gotSend == nil || svc.gotSend.TalentContactID.String() != contactID {
		t.Fatalf("send body not decoded: %+v", svc.gotSend)
	}
	// The handler passes the verified principal to the service (ReplyTo source).
	if svc.gotSendP.OrgID != p.OrgID || svc.gotSendP.Sub != p.Sub {
		t.Fatalf("principal not passed through: %+v", svc.gotSendP)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/bookings/offers", "")
	wantStatus(t, rec, http.StatusOK)
	offerID := uuid.NewString()
	rec = do(t, h, http.MethodGet, "/api/v1/bookings/offers/"+offerID, "")
	wantStatus(t, rec, http.StatusOK)

	rec = do(t, h, http.MethodGet, "/api/v1/bookings/suppressions", "")
	wantStatus(t, rec, http.StatusOK)
	rec = do(t, h, http.MethodPost, "/api/v1/bookings/suppressions", `{"email":"dj@example.com","reason":"stop"}`)
	wantStatus(t, rec, http.StatusCreated)
	if svc.gotSupp == nil || svc.gotSupp.Email != "dj@example.com" {
		t.Fatalf("suppression body not decoded: %+v", svc.gotSupp)
	}
	suppID := uuid.NewString()
	rec = do(t, h, http.MethodDelete, "/api/v1/bookings/suppressions/"+suppID, "")
	wantStatus(t, rec, http.StatusNoContent)
}

// ─── Error mapping (SPEC §12) ───────────────────────────────────────────────

func TestHandlerErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid", &InvalidError{Field: "name", Problem: "required"}, http.StatusUnprocessableEntity, "invalid"},
		{"no contact email", ErrNoContactEmail, http.StatusUnprocessableEntity, "no_contact_email"},
		{"no reply-to", ErrNoReplyTo, http.StatusUnprocessableEntity, "no_reply_to"},
		{"not found", ErrNotFound, http.StatusNotFound, "not_found"},
		{"forbidden", ErrForbidden, http.StatusForbidden, "forbidden"},
		{"duplicate", ErrDuplicate, http.StatusConflict, "duplicate"},
		{"conflict", ErrConflict, http.StatusConflict, "conflict"},
		{"suppressed", ErrSuppressed, http.StatusConflict, "suppressed"},
		{"rate limited", ErrRateLimited, http.StatusTooManyRequests, "rate_limited"},
		{"email unavailable", ErrEmailUnavailable, http.StatusServiceUnavailable, "email_unavailable"},
		{"internal", errors.New("boom"), http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := mount(t, &fakeService{err: tc.err}, ptrPrincipal(ownerPrincipal()))
			rec := do(t, h, http.MethodGet, "/api/v1/bookings/talent", "")
			wantError(t, rec, tc.status, tc.code)
		})
	}
}

func TestHandlerInvalidErrorCarriesField(t *testing.T) {
	h, _ := mount(t, &fakeService{err: &InvalidError{Field: "email", Problem: "must be a valid email address"}}, ptrPrincipal(ownerPrincipal()))
	rec := do(t, h, http.MethodGet, "/api/v1/bookings/talent", "")
	wantStatus(t, rec, http.StatusUnprocessableEntity)
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["field"] != "email" || body["problem"] == "" {
		t.Fatalf("422 must carry field + problem: %q", rec.Body.String())
	}
}

// ─── Body and param handling ─────────────────────────────────────────────────

func TestHandlerRejectsBadBodies(t *testing.T) {
	h, _ := mount(t, &fakeService{}, ptrPrincipal(ownerPrincipal()))

	rec := do(t, h, http.MethodPost, "/api/v1/bookings/talent", `{not json`)
	wantError(t, rec, http.StatusBadRequest, "invalid_body")

	// Unknown fields are rejected (strict decoding).
	rec = do(t, h, http.MethodPost, "/api/v1/bookings/talent", `{"name":"X","surprise":1}`)
	wantError(t, rec, http.StatusBadRequest, "invalid_body")

	// A malformed id in the path is a 404, not a 500.
	rec = do(t, h, http.MethodDelete, "/api/v1/bookings/talent/not-a-uuid", "")
	wantError(t, rec, http.StatusNotFound, "not_found")
}

// ─── Authz guard ─────────────────────────────────────────────────────────────

func TestHandlerRequiresPrincipal(t *testing.T) {
	h, _ := mount(t, &fakeService{}, nil) // no principal injected
	rec := do(t, h, http.MethodGet, "/api/v1/bookings/talent", "")
	wantStatus(t, rec, http.StatusUnauthorized)
}

func TestHandlerRoleSplit(t *testing.T) {
	booker := ownerPrincipal()
	booker.Roles = []string{"booker"}
	finance := ownerPrincipal()
	finance.Roles = []string{"finance"}
	door := ownerPrincipal()
	door.Roles = []string{"door"}
	door.EventScope = uuid.NewString()

	// Booker may read talent (talent.read is in the booker grants).
	h, _ := mount(t, &fakeService{}, ptrPrincipal(booker))
	wantStatus(t, do(t, h, http.MethodGet, "/api/v1/bookings/talent", ""), http.StatusOK)

	// Finance holds booking.read (artist_fee) but must NOT read talent PII.
	h, _ = mount(t, &fakeService{}, ptrPrincipal(finance))
	rec := do(t, h, http.MethodGet, "/api/v1/bookings/talent", "")
	wantStatus(t, rec, http.StatusForbidden)
	if !strings.Contains(rec.Body.String(), "no_role_grant") {
		t.Fatalf("finance denial reason: %q", rec.Body.String())
	}
	// …but finance may read the booking work (slots).
	wantStatus(t, do(t, h, http.MethodGet, "/api/v1/bookings/events/"+uuid.NewString()+"/slots", ""), http.StatusOK)

	// Door staff may not send offers.
	h, _ = mount(t, &fakeService{}, ptrPrincipal(door))
	rec = do(t, h, http.MethodPost, "/api/v1/bookings/offers", `{"talent_contact_id":"`+uuid.NewString()+`"}`)
	wantStatus(t, rec, http.StatusForbidden)

	// Marketing may not read talent PII either.
	marketing := ownerPrincipal()
	marketing.Roles = []string{"marketing"}
	h, _ = mount(t, &fakeService{}, ptrPrincipal(marketing))
	rec = do(t, h, http.MethodGet, "/api/v1/bookings/offers", "")
	wantStatus(t, rec, http.StatusForbidden)
	if !strings.Contains(rec.Body.String(), "no_role_grant") {
		t.Fatalf("marketing denial reason: %q", rec.Body.String())
	}
}
