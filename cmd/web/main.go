// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"github.com/palemoky/fight-the-landlord/internal/bot"
	"github.com/palemoky/fight-the-landlord/internal/webgame"
	"github.com/palemoky/fight-the-landlord/web"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := os.Getenv("WEB_ADDR")
	if addr == "" {
		addr = "127.0.0.1:9017"
	}
	origins := os.Getenv("WEB_ORIGINS")
	if origins == "" {
		origins = "http://127.0.0.1:9017,http://localhost:9017"
	}
	var engine bot.DecisionEngine = webgame.NewFallback()
	if url := os.Getenv("DOUZERO_URL"); url != "" {
		engine = bot.NewDouZeroEngineWithFallback(url, webgame.NewFallback())
	}
	s := webgame.New(strings.Split(origins, ","), engine, os.Getenv("TRUST_PROXY") == "true")
	server := &http.Server{Addr: addr, Handler: s.Handler(web.Files), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go s.RunCleanup(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Fengxun Dou Dizhu web listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
