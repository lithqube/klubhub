package release

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) handleListLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	links, err := h.svc.ListPlatformLinks(r.Context(), id)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": links})
}

func (h *Handler) handleSetLink(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	var body linkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	link, err := h.svc.SetPlatformLink(r.Context(), id, body.Platform, body.URL)
	if err != nil {
		h.writeStatusError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]interface{}{"data": link})
}

func (h *Handler) handleRemoveLink(w http.ResponseWriter, r *http.Request) {
	id, ok := h.releaseID(w, r)
	if !ok {
		return
	}
	platform := Platform(chi.URLParam(r, "platform"))
	if err := h.svc.RemovePlatformLink(r.Context(), id, platform); err != nil {
		h.writeStatusError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
