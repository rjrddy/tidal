package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/rjrddy/tidal/internal/replay"
	"github.com/rjrddy/tidal/internal/service"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8095", "HTTP address")
	bench := flag.Bool("bench", false, "run deterministic replay campaign")
	flag.Parse()
	if *bench {
		if e := replay.Campaign("docs/results/campaign.json"); e != nil {
			log.Fatal(e)
		}
		return
	}
	token := os.Getenv("TIDAL_TOKEN")
	if token == "" {
		token = "local-demo"
	}
	host, _, e := net.SplitHostPort(*addr)
	if e != nil {
		log.Fatal(e)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" && (token == "local-demo" || len(token) < 24) {
		log.Fatal("external binding requires private TIDAL_TOKEN and TLS proxy")
	}
	app, e := service.New(token, "apps/web/dist")
	if e != nil {
		log.Fatal(e)
	}
	defer app.Close()
	server := &http.Server{Addr: *addr, Handler: app, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	b, _ := json.Marshal(map[string]any{"event": "ready", "url": "http://" + *addr, "scope": "synthetic origin, live cache"})
	log.Print(string(b))
	if e := server.ListenAndServe(); e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
