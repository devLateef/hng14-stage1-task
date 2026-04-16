package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"name-profile-api/internal/enrichment"
	"name-profile-api/internal/handler"
	"name-profile-api/internal/repository"
	"name-profile-api/internal/service"
)

func main() {
	// Open SQLite database with WAL mode for concurrent read performance.
	db, err := sql.Open("sqlite", "profiles.db?_journal_mode=WAL")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	// Wire up dependencies.
	repo := repository.NewSQLiteRepository(db)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	enrichmentClient := enrichment.NewEnrichmentClient(httpClient)

	profileService := service.NewProfileService(repo, enrichmentClient)
	profileHandler := handler.NewProfileHandler(profileService)

	// Register routes.
	r := chi.NewRouter()
	r.Post("/api/profiles", profileHandler.CreateProfile)
	r.Get("/api/profiles", profileHandler.ListProfiles)
	r.Get("/api/profiles/{id}", profileHandler.GetProfile)
	r.Delete("/api/profiles/{id}", profileHandler.DeleteProfile)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
