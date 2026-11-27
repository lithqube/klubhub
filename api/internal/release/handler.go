package release

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ServiceIface is the slice of the release service the handler needs.
type ServiceIface interface {
	Create(ctx context.Context, in Input) (*Release, error)
	Update(ctx context.Context, id uuid.UUID, in Input) (*Release, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Transition(ctx context.Context, id uuid.UUID, to ReleaseStatus) (*Release, error)
	Get(ctx context.Context, id uuid.UUID) (*Release, error)
	List(ctx context.Context) ([]*Release, error)
	AddDeadline(ctx context.Context, releaseID uuid.UUID, name string, dueDate time.Time) (*Deadline, error)
	RenameDeadline(ctx context.Context, releaseID, id uuid.UUID, name string) (*Deadline, error)
	CompleteDeadline(ctx context.Context, releaseID, id uuid.UUID) (*Deadline, error)
	RemoveDeadline(ctx context.Context, releaseID, id uuid.UUID) error
	ListDeadlines(ctx context.Context, releaseID uuid.UUID) ([]*Deadline, error)
	SetPlatformLink(ctx context.Context, releaseID uuid.UUID, platform Platform, rawURL string) (*PlatformLink, error)
	RemovePlatformLink(ctx context.Context, releaseID uuid.UUID, platform Platform) error
	ListPlatformLinks(ctx context.Context, releaseID uuid.UUID) ([]*PlatformLink, error)
	AddChecklistItem(ctx context.Context, releaseID uuid.UUID, label string) (*ChecklistItem, error)
	RenameChecklistItem(ctx context.Context, releaseID, id uuid.UUID, label string) (*ChecklistItem, error)
	ToggleChecklistItem(ctx context.Context, releaseID, id uuid.UUID, done bool) (*ChecklistItem, error)
	RemoveChecklistItem(ctx context.Context, releaseID, id uuid.UUID) error
	ReorderChecklistItems(ctx context.Context, releaseID uuid.UUID, order []uuid.UUID) error
	SaveChecklistAsTemplate(ctx context.Context, releaseID uuid.UUID) error
	ListChecklistItems(ctx context.Context, releaseID uuid.UUID) ([]*ChecklistItem, error)
}

// Handler serves the release endpoints.
type Handler struct {
	svc ServiceIface
}

// NewHandler creates a Handler backed by the given service.
func NewHandler(svc ServiceIface) *Handler {
	return &Handler{svc: svc}
}

// Routes returns an http.Handler with all release routes registered.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/", h.handleList)
	r.Post("/", h.handleCreate)
	r.Get("/{id}", h.handleGet)
	r.Put("/{id}", h.handleUpdate)
	r.Delete("/{id}", h.handleDelete)
	r.Post("/{id}/transition", h.handleTransition)

	r.Get("/{id}/deadlines", h.handleListDeadlines)
	r.Post("/{id}/deadlines", h.handleAddDeadline)
	r.Put("/{id}/deadlines/{deadlineId}", h.handleRenameDeadline)
	r.Post("/{id}/deadlines/{deadlineId}/complete", h.handleCompleteDeadline)
	r.Delete("/{id}/deadlines/{deadlineId}", h.handleRemoveDeadline)

	r.Get("/{id}/links", h.handleListLinks)
	r.Post("/{id}/links", h.handleSetLink)
	r.Delete("/{id}/links/{platform}", h.handleRemoveLink)

	r.Get("/{id}/checklist", h.handleListChecklist)
	r.Post("/{id}/checklist", h.handleAddChecklistItem)
	r.Put("/{id}/checklist/{itemId}", h.handleRenameChecklistItem)
	r.Post("/{id}/checklist/{itemId}/toggle", h.handleToggleChecklistItem)
	r.Delete("/{id}/checklist/{itemId}", h.handleRemoveChecklistItem)
	r.Post("/{id}/checklist/reorder", h.handleReorderChecklist)
	r.Post("/{id}/checklist/save-as-default", h.handleSaveChecklistAsTemplate)

	return r
}

type createRequest struct {
	Title       string        `json:"title"`
	Artists     string        `json:"artists"`
	Label       string        `json:"label"`
	Distributor string        `json:"distributor"`
	ReleaseDate string        `json:"release_date"`
	Format      ReleaseFormat `json:"format"`
	Notes       string        `json:"notes"`
}

type transitionRequest struct {
	Status ReleaseStatus `json:"status"`
}

type deadlineRequest struct {
	Name    string `json:"name"`
	DueDate string `json:"due_date"`
}

type linkRequest struct {
	Platform Platform `json:"platform"`
	URL      string   `json:"url"`
}

type checklistRequest struct {
	Label string `json:"label"`
}

type toggleRequest struct {
	Done bool `json:"done"`
}

type reorderRequest struct {
	Order []uuid.UUID `json:"order"`
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	releases, err := h.svc.List(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": releases})
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var body createRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in, err := body.input()
	if err != nil {
		h.writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	rel, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{"data": rel})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	rel, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": rel})
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	var body createRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in, err := body.input()
	if err != nil {
		h.writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	rel, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": rel})
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.writeStatusError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleTransition(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	var body transitionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	rel, err := h.svc.Transition(r.Context(), id, body.Status)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": rel})
}

// releaseID reads and validates the {id} path parameter.
func (h *Handler) releaseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid release id")
		return uuid.Nil, false
	}
	return id, true
}

// deadlineID reads and validates the {deadlineId} path parameter.
func (h *Handler) deadlineID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "deadlineId"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid deadline id")
		return uuid.Nil, false
	}
	return id, true
}

// itemID reads and validates the {itemId} path parameter.
func (h *Handler) itemID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "itemId"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid checklist item id")
		return uuid.Nil, false
	}
	return id, true
}

// writeStatusError maps a service error to its HTTP status.
func (h *Handler) writeStatusError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		h.writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrInvalid):
		h.writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrInvalidTransition):
		h.writeError(w, http.StatusConflict, err.Error())
	default:
		h.writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}

// input turns the request body into a service input.
func (b createRequest) input() (Input, error) {
	date, err := time.Parse("2006-01-02", b.ReleaseDate)
	if err != nil {
		return Input{}, errors.New("release_date must be YYYY-MM-DD")
	}
	return Input{
		Title:       b.Title,
		Artists:     b.Artists,
		Label:       b.Label,
		Distributor: b.Distributor,
		ReleaseDate: date,
		Format:      b.Format,
		Notes:       b.Notes,
	}, nil
}
