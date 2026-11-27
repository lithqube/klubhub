package release

import (
	"encoding/json"
	"net/http"
	"time"
)

func (h *Handler) handleListDeadlines(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	deadlines, err := h.svc.ListDeadlines(r.Context(), id)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": deadlines})
}

func (h *Handler) handleAddDeadline(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	var body deadlineRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	due, err := time.Parse("2006-01-02", body.DueDate)
	if err != nil {
		h.writeError(w, http.StatusUnprocessableEntity, "due_date must be YYYY-MM-DD")
		return
	}
	deadline, err := h.svc.AddDeadline(r.Context(), id, body.Name, due)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{"data": deadline})
}

func (h *Handler) handleRenameDeadline(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	deadlineID, ok := h.deadlineID(w, r)
	if !ok {
		return
	}
	var body deadlineRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	deadline, err := h.svc.RenameDeadline(r.Context(), id, deadlineID, body.Name)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": deadline})
}

func (h *Handler) handleCompleteDeadline(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	deadlineID, ok := h.deadlineID(w, r)
	if !ok {
		return
	}
	deadline, err := h.svc.CompleteDeadline(r.Context(), id, deadlineID)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": deadline})
}

func (h *Handler) handleRemoveDeadline(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	deadlineID, ok := h.deadlineID(w, r)
	if !ok {
		return
	}
	if err := h.svc.RemoveDeadline(r.Context(), id, deadlineID); err != nil {
		h.writeStatusError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
