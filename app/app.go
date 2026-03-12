package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"kerkerker-douban-service/internal/config"
	"kerkerker-douban-service/internal/handler"
	"kerkerker-douban-service/internal/middleware"
	"kerkerker-douban-service/internal/repository"
	"kerkerker-douban-service/internal/service"
	"kerkerker-douban-service/pkg/httpclient"
	webassets "kerkerker-douban-service/web"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Application wraps the initialized runtime components.
type Application struct {
	Router  *gin.Engine
	cache   *repository.Cache
	metrics *repository.Metrics
}

// NewFromEnv creates an application by reading configuration from environment.
func NewFromEnv() (*Application, error) {
	return New(config.Load())
}

// New creates a fully initialized application router and dependencies.
func New(cfg *config.Config) (*Application, error) {
	gin.SetMode(cfg.GinMode)

	cache, err := repository.NewCache(cfg.RedisURL, 1*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	metrics, err := repository.NewMetrics(cfg.RedisURL)
	if err != nil {
		_ = cache.Close()
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}
	metrics.RecordServerStart(context.Background())
	log.Info().Msg("📊 Metrics enabled")

	httpClient := httpclient.NewClient(cfg.DoubanProxies)
	if httpClient.HasProxy() {
		log.Info().Int("count", httpClient.ProxyCount()).Msg("🔀 Proxy enabled")
	}

	doubanService := service.NewDoubanService(httpClient)
	tmdbService := service.NewTMDBService(cfg.TMDBAPIKeys, cfg.TMDBBaseURL, cfg.TMDBImageBase)
	if tmdbService.IsConfigured() {
		log.Info().Int("keys", tmdbService.KeyCount()).Msg("🎬 TMDB service enabled (轮询模式)")
	}

	heroHandler := handler.NewHeroHandler(doubanService, tmdbService, cache, cfg.CacheTTLHero)
	categoryHandler := handler.NewCategoryHandler(doubanService, cache)
	detailHandler := handler.NewDetailHandler(doubanService, cache)
	latestHandler := handler.NewLatestHandler(doubanService, cache)
	moviesHandler := handler.NewMoviesHandler(doubanService, cache)
	tvHandler := handler.NewTVHandler(doubanService, cache)
	newHandler := handler.NewNewHandler(doubanService, cache)
	searchHandler := handler.NewSearchHandler(doubanService, cache)
	adminHandler := handler.NewAdminHandler(doubanService, tmdbService, metrics)
	calendarHandler := handler.NewCalendarHandler(tmdbService, doubanService, cache, cfg.CacheTTLCategory)

	indexHTML, err := webassets.ReadIndexHTML()
	if err != nil {
		_ = metrics.Close()
		_ = cache.Close()
		return nil, fmt.Errorf("failed to load embedded admin page: %w", err)
	}
	staticFiles, err := webassets.StaticFS()
	if err != nil {
		_ = metrics.Close()
		_ = cache.Close()
		return nil, fmt.Errorf("failed to load embedded static files: %w", err)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logging())
	r.Use(middleware.Metrics(metrics))
	r.Use(middleware.CORS())

	serveIndex := func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}
	r.GET("/", serveIndex)
	r.GET("/admin", serveIndex)
	r.StaticFS("/static", http.FS(staticFiles))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Unix(),
		})
	})

	api := r.Group("/api/v1")
	{
		api.GET("/status", adminHandler.GetStatus)
		api.GET("/hero", heroHandler.GetHero)
		api.GET("/category", categoryHandler.GetCategory)
		api.GET("/detail/:id", detailHandler.GetDetail)
		api.GET("/latest", latestHandler.GetLatest)
		api.GET("/movies", moviesHandler.GetMovies)
		api.GET("/tv", tvHandler.GetTV)
		api.GET("/new", newHandler.GetNew)
		api.GET("/search", searchHandler.Search)
		api.POST("/search", searchHandler.GetSearchTags)
		api.GET("/calendar", calendarHandler.GetCalendar)
		api.GET("/calendar/airing", calendarHandler.GetAiring)
	}

	admin := r.Group("/api/v1")
	admin.Use(middleware.AdminAuth(cfg.AdminAPIKey))
	{
		admin.GET("/analytics", adminHandler.GetAnalytics)
		admin.GET("/analytics/endpoint", adminHandler.GetEndpointStats)
		admin.DELETE("/analytics", adminHandler.ResetAnalytics)

		admin.DELETE("/hero", heroHandler.DeleteHeroCache)
		admin.DELETE("/category", categoryHandler.DeleteCategoryCache)
		admin.DELETE("/detail/:id", detailHandler.DeleteDetailCache)
		admin.DELETE("/detail", detailHandler.DeleteAllDetailCache)
		admin.DELETE("/latest", latestHandler.DeleteLatestCache)
		admin.DELETE("/movies", moviesHandler.DeleteMoviesCache)
		admin.DELETE("/tv", tvHandler.DeleteTVCache)
		admin.DELETE("/new", newHandler.DeleteNewCache)
		admin.DELETE("/search", searchHandler.DeleteSearchCache)
		admin.DELETE("/calendar", calendarHandler.DeleteCalendarCache)
	}

	if cfg.AdminAPIKey != "" {
		log.Info().Msg("🔐 Admin API 认证已启用")
	} else {
		log.Warn().Msg("⚠️  Admin API 未配置认证，管理接口对外开放")
	}

	return &Application{
		Router:  r,
		cache:   cache,
		metrics: metrics,
	}, nil
}

// Close releases application dependencies.
func (a *Application) Close() error {
	var closeErr error

	if a.metrics != nil {
		if err := a.metrics.Close(); err != nil {
			closeErr = err
		}
	}
	if a.cache != nil {
		if err := a.cache.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}

	return closeErr
}
