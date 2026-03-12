package handler

import (
	"net/http"
	"strings"
	"sync"

	"kerkerker-douban-service/app"

	"github.com/rs/zerolog/log"
)

var (
	initMu      sync.Mutex
	router      http.Handler
	bootstrapErr error
)

func initialize() {
	bootstrapErr = nil

	application, err := app.NewFromEnv()
	if err != nil {
		bootstrapErr = err
		return
	}

	router = application.Router
}

func getRouter() (http.Handler, error) {
	if router != nil {
		return router, nil
	}

	initMu.Lock()
	defer initMu.Unlock()

	if router != nil {
		return router, nil
	}

	initialize()
	if bootstrapErr != nil {
		return nil, bootstrapErr
	}

	return router, nil
}

// Handler is the Vercel Go Function entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	h, err := getRouter()
	if err != nil {
		log.Error().Err(err).Msg("Failed to bootstrap Vercel handler")
		http.Error(w, "service initialization failed", http.StatusInternalServerError)
		return
	}

	restoreOriginalPath(r)
	h.ServeHTTP(w, r)
}

func restoreOriginalPath(r *http.Request) {
	query := r.URL.Query()
	original := strings.TrimSpace(query.Get("__pathname"))
	if original == "" {
		return
	}

	if !strings.HasPrefix(original, "/") {
		original = "/" + original
	}
	r.URL.Path = original

	query.Del("__pathname")
	r.URL.RawQuery = query.Encode()
	if r.URL.RawQuery == "" {
		r.URL.ForceQuery = false
	}

	// Keep RequestURI aligned for middlewares reading raw URI.
	if r.URL.RawQuery == "" {
		r.RequestURI = r.URL.Path
		return
	}
	r.RequestURI = r.URL.Path + "?" + r.URL.RawQuery
}
