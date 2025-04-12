package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/iahta/chirpy/internal/auth"
	"github.com/iahta/chirpy/internal/database"
)

type User struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
}

func (cfg *apiConfig) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	type response struct {
		ID          uuid.UUID `json:"id"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		Email       string    `json:"email"`
		IsChirpyRed bool      `json:"is_chirpy_red"`
	}

	authHeader, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	userID, err := auth.ValidateJWT(authHeader, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid Credentials")
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding json: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}
	if !isValidEmail(params.Email) {
		log.Printf("Invalid email: %s", params.Email)
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	newPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create new password")
		return
	}

	updatedUser, err := cfg.database.UpdatePasswordEmailUser(r.Context(), database.UpdatePasswordEmailUserParams{
		Email:          params.Email,
		UpdatedAt:      time.Now(),
		HashedPassword: newPassword,
		ID:             userID,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update user profile")
		return
	}

	respondWithJSON(w, http.StatusOK, response{
		ID:          updatedUser.ID,
		CreatedAt:   updatedUser.CreatedAt,
		UpdatedAt:   updatedUser.UpdatedAt,
		Email:       updatedUser.Email,
		IsChirpyRed: updatedUser.IsChirpyRed.Bool,
	})

}

func (cfg *apiConfig) upgradeUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}

	authKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized request")
		return
	}

	err = auth.ValidatePolkaKey(authKey, cfg.polkaKey)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized request")
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding json: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}
	if params.Event != "user.upgraded" {
		respondWithJSON(w, http.StatusNoContent, nil)
		return
	}
	///convert to uuid
	parsedUser, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Invalid userID format. Ensure it is a valid UUID")
		return
	}

	isAlreadyChirpyRed, err := cfg.database.IsUserChirpyRed(r.Context(), parsedUser)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "User can't be found")
		return
	}
	if !isAlreadyChirpyRed.Bool {
		err = cfg.database.UpgradeUserToRed(r.Context(), parsedUser)
		if err != nil {
			respondWithError(w, http.StatusNotFound, "User can't be found")
			return
		}
	}

	respondWithJSON(w, http.StatusNoContent, nil)

}
