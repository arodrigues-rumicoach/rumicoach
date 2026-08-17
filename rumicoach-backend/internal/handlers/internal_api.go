package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/regional"
	"go.uber.org/zap"
)

// RegisterInternalRoutes mounts the server-to-server API. All routes are protected
// by regional.AuthMiddleware (Google-signed service identity tokens).
func RegisterInternalRoutes(r chi.Router, s *Server, isAuthPlane bool) {
	r.Route("/internal", func(r chi.Router) {
		r.Use(regional.AuthMiddleware)
		// Data plane: the auth plane provisions profiles for users whose residency is here.
		r.Post("/users", s.ProvisionInternalUser)
		r.Put("/users/{userID}/contact", s.UpdateInternalUserContact)
		if isAuthPlane {
			// Auth plane: data planes request identity erasure after deleting local data.
			r.Delete("/identities/{userID}", s.DeleteInternalIdentity)
		}
	})
}

// ProvisionInternalUser creates the local profile row for a user whose identity
// lives on the auth plane. Idempotent: re-provisioning an existing ID is a no-op.
func (s *Server) ProvisionInternalUser(w http.ResponseWriter, r *http.Request) {
	var p regional.UserProvision
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, `{"error": "invalid payload"}`, http.StatusBadRequest)
		return
	}
	if p.ID == "" {
		http.Error(w, `{"error": "id is required"}`, http.StatusBadRequest)
		return
	}

	if err := regional.CreateLocalProfile(p); err != nil {
		s.logger.Error("provision create failed", zap.String("user_id", p.ID), zap.Error(err))
		http.Error(w, `{"error": "failed to provision user"}`, http.StatusInternalServerError)
		return
	}

	s.logger.Info("provisioned regional profile", zap.String("user_id", p.ID), zap.String("data_region", p.DataRegion))
	w.WriteHeader(http.StatusCreated)
}

// UpdateInternalUserContact updates the contact info for a local profile
func (s *Server) UpdateInternalUserContact(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		http.Error(w, `{"error": "user id required"}`, http.StatusBadRequest)
		return
	}

	var p regional.UserContactUpdate
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, `{"error": "invalid payload"}`, http.StatusBadRequest)
		return
	}
	p.ID = userID

	if err := regional.UpdateLocalUserContact(p); err != nil {
		s.logger.Error("contact update failed", zap.String("user_id", p.ID), zap.Error(err))
		http.Error(w, `{"error": "failed to update user contact"}`, http.StatusInternalServerError)
		return
	}

	s.logger.Info("updated regional contact info", zap.String("user_id", p.ID))
	w.WriteHeader(http.StatusOK)
}

// DeleteInternalIdentity anonymizes a user's identity record on the auth plane,
// mirroring what DeleteCurrentUser does to profile rows. Idempotent.
func (s *Server) DeleteInternalIdentity(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		http.Error(w, `{"error": "user id required"}`, http.StatusBadRequest)
		return
	}

	if err := eraseIdentity(userID); err != nil {
		s.logger.Error("identity erasure failed", zap.String("user_id", userID), zap.Error(err))
		http.Error(w, `{"error": "failed to erase identity"}`, http.StatusInternalServerError)
		return
	}

	s.logger.Info("erased identity record", zap.String("user_id", userID))
	w.WriteHeader(http.StatusNoContent)
}

// eraseIdentity anonymizes an identity record so the account can no longer log in.
// Idempotent; used by the internal endpoint and by local deletions on the auth plane.
func eraseIdentity(userID string) error {
	anonEmail := fmt.Sprintf("%s@anonymized.com", userID)
	deletedName := "Deleted User"
	updates := map[string]interface{}{
		"email":                    anonEmail,
		"phone_number":             nil,
		"name":                     &deletedName,
		"google_id":                nil,
		"apple_id":                 nil,
		"refresh_token_hash":       nil,
		"refresh_token_expires_at": nil,
		"is_active":                false,
		// A never-activated waitlist identity carries its profile payload here — PII
		// that must go with the rest.
		"pending_provision": nil,
		"deleted_at":        time.Now(),
	}
	return database.Auth().Model(&models.Identity{}).Where("id = ?", userID).Updates(updates).Error
}
