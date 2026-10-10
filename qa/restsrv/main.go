// Команда restsrv — минимальный хост REST /api/v1 для сквозных сценарных
// прогонов (qa/runner). Модуль по построению не запускает серверов
// (docs/plan/04-module-functionality.md §5) — хост всегда обязанность
// встраивающей стороны; этот хост — её dev-заглушка только для QA.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/billydos/components-catalog/internal/httpapi"
	"github.com/billydos/components-catalog/internal/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "restsrv: %v\n", err)
		os.Exit(1)
	}
}

// run — тело main: отложенные закрытия (приложение, слушатель) выполняются
// до выхода с ненулевым кодом — os.Exit только здесь, в main.
func run() error {
	addr := flag.String("addr", "127.0.0.1:8080", "адрес прослушивания (порт 0 — выбрать свободный)")
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
		return err
	}
	defer app.Close() //nolint:errcheck — закрытие при выходе

	// Порт выбирается здесь и держится до Serve — прогонщик передаёт
	// --addr=127.0.0.1:0 и разбирает фактический адрес из строки
	// готовности ниже.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           httpapi.New(app, httpapi.Config{}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "restsrv: ready http://%s/api/v1 (%s)\n", ln.Addr(), *dialect)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
