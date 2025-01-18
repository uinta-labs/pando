package pkg

import (
	"log"

	"github.com/caarlos0/env/v10"
	"github.com/pkg/errors"
)

type Config struct {
	Debug             bool     `env:"DEBUG" envDefault:"false"`
	DatabaseURL       string   `env:"DATABASE_URL" envDefault:"postgres://postgres:postgres@localhost:5332/postgres?sslmode=disable"`
	Host              string   `env:"HOST" envDefault:"0.0.0.0"`
	Port              string   `env:"PORT" envDefault:"5900"`
	AuthorizedOrigins []string `env:"AUTHORIZED_ORIGINS" envDefault:"http://localhost:3000,https://buf.build,https://graphene.fluffy-broadnose.ts.net"`
	SentryDSN         string   `env:"SENTRY_DSN" envDefault:""`
	Environment       string   `env:"ENVIRONMENT" envDefault:"development"`

	// When set, all HTTP requests will be authenticated as this user, regardless of the actual token (or lack thereof)
	DevelopmentAuthUserEmail string `env:"DEVELOPMENT_AUTH_USER_EMAIL" envDefault:""`
}

func ReadConfig() (Config, error) {
	cfg := Config{}
	parseErr := env.Parse(&cfg)
	if parseErr != nil {
		return cfg, errors.Wrap(parseErr, "failed to parse environment variables")
	}

	if cfg.DevelopmentAuthUserEmail != "" {
		if !cfg.Debug {
			return cfg, errors.New("DEVELOPMENT_AUTH_USER_EMAIL can only be set when DEBUG is true")
		}
		if cfg.Environment != "development" {
			return cfg, errors.New("DEVELOPMENT_AUTH_USER_EMAIL can only be set when ENVIRONMENT is development")
		}

		log.Printf("!!!\n!!! DEVELOPMENT_AUTH_USER_EMAIL is set to %s\n!!!", cfg.DevelopmentAuthUserEmail)
	}

	return cfg, nil
}
