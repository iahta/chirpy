package main

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/iahta/chirpy/internal/auth"
)

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) deleteChirpHandler(w http.ResponseWriter, r *http.Request) {
	authHeader, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing Authorization")
		return
	}
	userID, err := auth.ValidateJWT(authHeader, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusForbidden, "Invalid Credentials")
		return
	}
	chirpID := r.PathValue("chirpID")
	if chirpID == "" {
		respondWithError(w, http.StatusNotFound, "Chirp ID is missing in the request path")
		return
	}
	parsedChirp, err := uuid.Parse(chirpID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Invalid chirpID format. Ensure it is a valid UUID")
		return
	}
	chirp, err := cfg.database.GrabChirp(r.Context(), parsedChirp)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Chirp Not found")
		return
	}
	if chirp.UserID != userID {
		respondWithError(w, http.StatusForbidden, "Only chirp authors can delete chirps")
		return
	}
	err = cfg.database.DeleteChirp(r.Context(), parsedChirp)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Unable to delete chirp")
		return
	}
	respondWithJSON(w, http.StatusNoContent, nil)
}
