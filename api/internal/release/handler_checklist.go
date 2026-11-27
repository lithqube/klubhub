package release

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) handleListChecklist(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListChecklistItems(r.Context(), id)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": items})
}

func (h *Handler) handleAddChecklistItem(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	var body checklistRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	item, err := h.svc.AddChecklistItem(r.Context(), id, body.Label)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{"data": item})
}

func (h *Handler) handleRenameChecklistItem(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	itemID, ok := h.itemID(w, r)
	if !ok {
		return
	}
	var body checklistRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	item, err := h.svc.RenameChecklistItem(r.Context(), id, itemID, body.Label)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": item})
}

func (h *Handler) handleToggleChecklistItem(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	itemID, ok := h.itemID(w, r)
	if !ok {
		return
	}
	var body toggleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	item, err := h.svc.ToggleChecklistItem(r.Context(), id, itemID, body.Done)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": item})
}

func (h *Handler) handleRemoveChecklistItem(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	itemID, ok := h.itemID(w, r)
	if !ok {
		return
	}
	if err := h.svc.RemoveChecklistItem(r.Context(), id, itemID); err != nil {
		h.writeStatusError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleReorderChecklist(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	var body reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.svc.ReorderChecklistItems(r.Context(), id, body.Order); err != nil {
		h.writeStatusError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSaveChecklistAsTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.SaveChecklistAsTemplate(r.Context(), id); err != nil {
		h.writeStatusError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
