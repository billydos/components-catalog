// Команда restsrv — минимальный хост REST /api/v1 для сквозных сценарных
// прогонов этапа 7.1 (qa/scenarios.sh). Модуль по построению не запускает
// серверов (docs/plan/04-module-functionality.md §5) — хост всегда обязанность
// встраивающей стороны; этот хост — её-dev-заглушка только для QA.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/billydos/components-catalog/internal/httpapi"
	"github.com/billydos/components-catalog/internal/service"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "адрес прослушивания")
	dialect := flag.String("dialect", "sqlite", "диалект хранилища: sqlite | postgres")
	dsn := flag.String("dsn", "", "DSN postgres (приоритетнее -db)")
	db := flag.String("db", "", "путь файла sqlite (по умолчанию catalog.db)")
	ensure := flag.Bool("ensure", false, "DDL и сиды каталога на пустую базу при старте")
	flag.Parse()

	dsnValue := *dsn
	if dsnValue == "" {
		dsnValue = *db
	}
	if dsnValue == "" {
		dsnValue = "catalog.db"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := service.Open(ctx, service.Config{
		Dialect: *dialect, DSN: dsnValue, EnsureCreated: *ensure,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "restsrv: %v\n", err)
		os.Exit(1)
	}
	defer app.Close() //nolint:errcheck — закрытие при выходе

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.New(app, httpapi.Config{}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "restsrv: http://%s/api/v1 (%s)\n", *addr, *dialect)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "restsrv: %v\n", err)
		os.Exit(1)
	}
}
