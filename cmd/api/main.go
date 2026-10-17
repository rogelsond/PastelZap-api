package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"pedidos-api/internal/config"
	"pedidos-api/internal/db"
	"pedidos-api/internal/httpapi"
	"pedidos-api/internal/store"
	"strconv"
	"syscall"
	"time"
)

func main() {
	cfg, e := config.Load(os.Getenv)
	if e != nil {
		slog.Error("configuração inválida", "error", e)
		os.Exit(1)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	p, e := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if e != nil {
		log.Error("erro ao conectar banco", "error", e)
		os.Exit(1)
	}
	defer p.Close()
	if e = db.Migrate(ctx, p); e != nil {
		log.Error("erro nas migrações", "error", e)
		os.Exit(1)
	}
	s := &http.Server{Addr: ":" + strconv.Itoa(cfg.Port), Handler: httpapi.NewHandler(store.New(p), cfg.APIToken, log), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		log.Info("servidor iniciado", "port", cfg.Port)
		if e := s.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Error("servidor falhou", "error", e)
			stop()
		}
	}()
	<-ctx.Done()
	log.Info("encerrando")
	x, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	if e := s.Shutdown(x); e != nil {
		log.Error("erro ao encerrar", "error", e)
	}
}

var _ httpapi.Store = (*store.Store)(nil)
