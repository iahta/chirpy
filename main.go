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

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	database       *database.Queries
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Unable to call database: %v", err)
	}
	ok := []byte("OK")
	apiCfg := apiConfig{}

	dbQueries := database.New(db)
	apiCfg.database = dbQueries

	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))

	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(appHandler))

	mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	mux.HandleFunc("POST /api/validate_chirp", apiCfg.validateHandler)

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
	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Counter Reset!")
}

func (cfg *apiConfig) validateHandler(w http.ResponseWriter, r *http.Request) {
	type validate struct {
		Body string `json:"body"`
	}

	type cleanedResponse struct {
		CleanedBody string `json:"cleaned_body"`
	}

	decoder := json.NewDecoder(r.Body)
	val := validate{}
	err := decoder.Decode(&val)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}
	if len(val.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}
	cleanedText := filterProfanity(val.Body)

	resp := cleanedResponse{CleanedBody: cleanedText}
	respondWithJSON(w, http.StatusOK, resp)
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
