package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

type Config struct {
	Port                  int
	DatabaseURL, APIToken string
	DBMaxConns            int32
	LogLevel              slog.Level
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{Port: 8080, DBMaxConns: 4, LogLevel: slog.LevelInfo}
	var es []error
	c.DatabaseURL = strings.TrimSpace(getenv("DATABASE_URL"))
	c.APIToken = strings.TrimSpace(getenv("API_TOKEN"))
	if c.DatabaseURL == "" {
		es = append(es, errors.New("DATABASE_URL é obrigatória"))
	}
	if c.APIToken == "" {
		es = append(es, errors.New("API_TOKEN é obrigatória"))
	} else if len(c.APIToken) < 32 {
		es = append(es, errors.New("API_TOKEN deve ter pelo menos 32 caracteres"))
	}
	if s := strings.TrimSpace(getenv("PORT")); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 || n > 65535 {
			es = append(es, errors.New("PORT inválida"))
		} else {
			c.Port = n
		}
	}
	if s := strings.TrimSpace(getenv("DB_MAX_CONNS")); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 || n > 20 {
			es = append(es, errors.New("DB_MAX_CONNS deve ser um inteiro entre 1 e 20"))
		} else {
			c.DBMaxConns = int32(n)
		}
	}
	switch strings.TrimSpace(getenv("LOG_LEVEL")) {
	case "", "info":
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		es = append(es, fmt.Errorf("LOG_LEVEL deve ser debug, info, warn ou error"))
	}
	return c, errors.Join(es...)
}
