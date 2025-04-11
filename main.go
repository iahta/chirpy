package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/iahta/chirpy/internal/auth"
	"github.com/iahta/chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	database       *database.Queries
	platform       string
	jwtSecret      string
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	JWT_Secret := os.Getenv("JWT_SECRET")
	if platform == "" {
		log.Fatal("PLATFORM must be set")
	}
	if JWT_Secret == "" {
		log.Fatal("JWT_SECRET must be set")
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Unable to call database: %v", err)
	}
	dbQueries := database.New(db)
	ok := []byte("OK")
	apiCfg := apiConfig{
		fileserverHits: atomic.Int32{},
		database:       dbQueries,
		platform:       platform,
		jwtSecret:      JWT_Secret,
	}

	mux := http.NewServeMux()
	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(appHandler))

	mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	mux.HandleFunc("GET /api/chirps", apiCfg.retrieveHandler)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.grabChirpHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	mux.HandleFunc("POST /api/chirps", apiCfg.createChirpHandler)
	mux.HandleFunc("POST /api/users", apiCfg.handlerUsers)
	mux.HandleFunc("POST /api/login", apiCfg.loginHandler)
	mux.HandleFunc("POST /api/refresh", apiCfg.refreshHandler)
	mux.HandleFunc("POST /api/revoke", apiCfg.revokeHandler)

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(ok)
	})

	// Wrap the `mux` with `middlewareLog`
	//wrappedMux := middlewareLog(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	server.ListenAndServe()
}

func (cfg *apiConfig) revokeHandler(w http.ResponseWriter, r *http.Request) {
	authHeader, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	refresh_token, err := cfg.database.GetRefreshToken(r.Context(), authHeader)
	if err != nil || refresh_token.Token == "" {
		respondWithError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}
	err = cfg.database.UpdateRefreshToken(r.Context(), database.UpdateRefreshTokenParams{
		UpdatedAt: time.Now(),
		RevokedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
		Token: refresh_token.Token,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to revoke refresh token")
		return
	}
	respondWithJSON(w, http.StatusNoContent, nil)

}

func (cfg *apiConfig) refreshHandler(w http.ResponseWriter, r *http.Request) {
	authHeader, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	refresh_token, err := cfg.database.GetRefreshToken(r.Context(), authHeader)
	if err != nil || refresh_token.Token == "" {
		respondWithError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}
	if time.Now().After(refresh_token.ExpiresAt) || refresh_token.RevokedAt.Valid {
		respondWithError(w, http.StatusUnauthorized, "Refresh token is invalid")
		return
	}
	user, err := cfg.database.GetUserById(r.Context(), refresh_token.UserID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to retrieve User")
		return
	}

	exp := time.Duration(3600 * time.Second)
	token, err := auth.MakeJWT(user.ID, cfg.jwtSecret, exp)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create new token")
		return
	}

	type response struct {
		Token string `json:"token"`
	}
	respondWithJSON(w, http.StatusOK, response{
		Token: token,
	})

}

/* create refresh struct in own file, to grab refresh token do is revoked too//auto sqlc generated function to get refresh token, not with a pointer to struct
func (t RefreshToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}
*/

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	type response struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
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
	user, err := cfg.database.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}
	err = auth.CheckPasswordHash(user.HashedPassword, params.Password)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}
	exp := time.Duration(3600) * time.Second
	token, err := auth.MakeJWT(user.ID, cfg.jwtSecret, exp)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create authentication token")
		return
	}

	refresh_token, err := auth.MakeRefreshToken()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Coludn't create refresh token")
		return
	}
	cfg.database.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refresh_token,
		UserID:    user.ID,
		ExpiresAt: time.Now().AddDate(0, 0, 60),
	})

	respondWithJSON(w, http.StatusOK, response{
		ID:           user.ID,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Email:        user.Email,
		Token:        token,
		RefreshToken: refresh_token,
	})

}

func (cfg *apiConfig) handlerUsers(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	type response struct {
		User
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding json: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}
	if !isValidEmail(params.Email) {
		log.Printf("Invalid email: %s", params.Email)
		respondWithError(w, http.StatusBadRequest, "Invalid Email format")
		return
	}
	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("failed to hash password %v", err)
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	user, err := cfg.database.CreateUser(r.Context(), database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		log.Printf("failed to create user: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	respondWithJSON(w, http.StatusCreated, response{
		User: User{
			ID:        user.ID,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
			Email:     user.Email,
		},
	})
}

func isValidEmail(email string) bool {
	return strings.Contains(email, "@")
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//increment the counter
		cfg.fileserverHits.Add(1)
		//call the next handler
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w,
		"<html> <body> <h1>Welcome, Chirpy Admin</h1> <p>Chirpy has been visited %d times!</p></body> </html>",
		cfg.fileserverHits.Load())
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Reset is only allowed in dev environment."))
		return
	}

	cfg.fileserverHits.Store(0)
	cfg.database.DeleteUsers(r.Context())
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset to 0 and database reset to initial state."))
}

func (cfg *apiConfig) retrieveHandler(w http.ResponseWriter, r *http.Request) {
	chirpsArray, err := cfg.database.RetrieveChirps(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve chirps")
		return
	}
	//convert database struct to api struct (lower case field names)
	chirpsToReturn := make([]Chirp, len(chirpsArray))
	for i, c := range chirpsArray {
		chirpsToReturn[i] = Chirp{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			UserID:    c.UserID,
		}
	}

	respondWithJSON(w, http.StatusOK, chirpsToReturn)
}

func (cfg *apiConfig) grabChirpHandler(w http.ResponseWriter, r *http.Request) {
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
		respondWithError(w, http.StatusNotFound, "Chirp not found")
		return
	}
	response := Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	respondWithJSON(w, http.StatusOK, response)
}

func (cfg *apiConfig) createChirpHandler(w http.ResponseWriter, r *http.Request) {
	type validate struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	val := validate{}
	err := decoder.Decode(&val)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, http.StatusBadRequest, "Invalid Json")
		return
	}
	if len(val.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}
	if len(val.Body) == 0 {
		respondWithError(w, http.StatusBadRequest, "Chirp body is empty")
		return
	}
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	cleanedText := filterProfanity(val.Body)

	createdChirp, err := cfg.database.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleanedText,
		UserID: userID,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create chirp")
		return
	}
	response := Chirp{
		ID:        createdChirp.ID,
		CreatedAt: createdChirp.CreatedAt,
		UpdatedAt: createdChirp.UpdatedAt,
		Body:      createdChirp.Body,
		UserID:    createdChirp.UserID,
	}
	respondWithJSON(w, http.StatusCreated, response)
}

func filterProfanity(chirp string) string {
	bad_words := []string{"kerfuffle", "sharbert", "fornax"}
	split := strings.Split(chirp, " ")
	for i, word := range split {
		lower_cased := strings.ToLower(word)
		for _, bad_word := range bad_words {
			if lower_cased == bad_word {
				split[i] = "****"
			}
		}
	}
	joined := strings.Join(split, " ")
	return joined
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorResponse struct {
		Error string `json:"error"`
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	errResp := errorResponse{Error: msg}
	dat, _ := json.Marshal(errResp)
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	dat, err := json.Marshal(payload)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error marshalling JSON")
		return
	}
	w.Write(dat)
}

/*
func middlewareLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
*/

/*
// Convert a database Chirp to an API Chirp
func dbChirpToAPIChirp(dbChirp database.Chirp) Chirp {
    return Chirp{
        ID:        dbChirp.ID,
        CreatedAt: dbChirp.CreatedAt,
        UpdatedAt: dbChirp.UpdatedAt,
        Body:      dbChirp.Body,
        UserID:    dbChirp.UserID,
    }
}

// Convert a slice of database Chirps to API Chirps
func dbChirpsToAPIChirps(dbChirps []database.Chirp) []Chirp {
    apiChirps := make([]Chirp, len(dbChirps))
    for i, dbChirp := range dbChirps {
        apiChirps[i] = dbChirpToAPIChirp(dbChirp)
    }
    return apiChirps
}
Datbase chirp conversion becomes much cleaner
func (cfg *apiConfig) retrieveHandler(w http.ResponseWriter, r *http.Request) {
    dbChirps, err := cfg.database.RetrieveChirps(r.Context())
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve chirps")
        return
    }

    // Much cleaner with a helper function!
    apiChirps := dbChirpsToAPIChirps(dbChirps)
    respondWithJSON(w, http.StatusOK, apiChirps)
}

*/
