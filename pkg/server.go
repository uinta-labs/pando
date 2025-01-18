package pkg

import (
	connectcors "connectrpc.com/cors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/cors"

	database "github.com/uinta-labs/pando/pkg/db"
)

type Server struct {
	cfg        Config
	db         *database.DB
	corsConfig *cors.Cors
}

func NewServer(cfg Config, db *database.DB) (*Server, error) {

	corsConfig := cors.New(cors.Options{
		AllowedOrigins:   cfg.AuthorizedOrigins,
		ExposedHeaders:   connectcors.ExposedHeaders(),
		AllowedMethods:   connectcors.AllowedMethods(),
		AllowCredentials: true,
		AllowedHeaders: []string{
			"Authorization",
			"Baggage",
			"Connect-Protocol-Version",
			"Content-Type",
			"Cookie",
			"Origin",
			"Sentry-Trace",
			"User-Agent",
			"Baggage",
		},
		Debug: cfg.Debug,
	})

	return &Server{
		cfg:        cfg,
		db:         db,
		corsConfig: corsConfig,
	}, nil
}
