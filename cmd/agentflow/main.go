package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"agentflow/internal/config"
	"agentflow/internal/httpapi"
	"agentflow/internal/repo"
	"agentflow/internal/tracker"

	"github.com/gin-gonic/gin"
)

// loadDotEnv loads KEY=VALUE pairs from the first .env file found. Search
// order: the current working directory and up to three parents (so running
// from a subdirectory still finds the project root), then next to the
// executable. Existing environment variables take precedence and are never
// overwritten.
func loadDotEnv() {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 4; i++ {
			candidates = append(candidates, filepath.Join(dir, ".env"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), ".env"))
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if k != "" && os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
		return
	}
}

func main() {
	addr := flag.String("addr", "", "HTTP listen address; overrides AGENTFLOW_HTTP_PORT")
	basicUser := flag.String("basic-user", "", "optional HTTP Basic Auth username")
	basicPass := flag.String("basic-pass", "", "optional HTTP Basic Auth password")
	tlsCert := flag.String("tls-cert", "", "optional TLS certificate path")
	tlsKey := flag.String("tls-key", "", "optional TLS private key path")
	datasource := flag.String("datasource", "", "platform datasource: a SQLite file path or \"file:\" URI, or a MySQL DSN; overrides AGENTFLOW_DATASOURCE_DSN and AGENTFLOW_MYSQL_DSN")
	cliAction := flag.String("cli-action", "", "local managed CLI administration: status, update, or rollback; exits without starting services")
	cliProvider := flag.String("cli-provider", "", "managed CLI provider to roll back")
	cliTimeout := flag.Duration("cli-timeout", 80*time.Minute, "total managed CLI operation timeout")
	flag.Parse()
	loadDotEnv()
	if os.Getenv("AGENTFLOW_SESSION_SECRET") == "" && *cliAction == "" {
		log.Fatal("AGENTFLOW_SESSION_SECRET is required; generate with openssl rand -hex 32")
	}
	if os.Getenv("AGENTFLOW_AUTH_REQUIRED") == "" {
		os.Setenv("AGENTFLOW_AUTH_REQUIRED", "true")
	}
	if *cliAction != "" || *cliProvider != "" {
		if flag.NArg() != 0 {
			log.Fatal("CLI administration does not accept positional arguments")
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		err := runCLI(ctx, *cliAction, *cliProvider, *cliTimeout, os.Stdout)
		stop()
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	s, err := repo.Open(strings.TrimSpace(*datasource))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("storage driver=%v database=%v", s.StorageInfo()["driver"], s.StorageInfo()["database"])
	endpoints, err := config.LoadEndpoints("api", strings.TrimSpace(*addr))
	if err != nil {
		log.Fatal(err)
	}
	config.SetCurrent(endpoints)

	mainLn, inherited, err := listenMain(endpoints.ListenAddr)
	if err != nil {
		log.Fatal(err)
	}
	syncSvc := tracker.NewService()
	syncSvc.ConfigureWithStore(s)
	gin.SetMode(gin.ReleaseMode)

	handler := httpapi.New(s, syncSvc, *basicUser, *basicPass)
	srv := &http.Server{Addr: endpoints.ListenAddr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	readyStatus := handoverStatus
	if inherited {
		log.Printf("taking over listener %s from pid %d", mainLn.Addr(), os.Getppid())
	}

	var handedOver atomic.Bool
	stopped := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	wait:
		for received := range sig {
			if received != syscall.SIGHUP {
				log.Printf("shutting down on %s", received)
				break
			}
			log.Printf("handing listener %s over to a new process...", mainLn.Addr())
			successor, err := handover(mainLn, handoverReadyTimeout)
			if err != nil {
				log.Printf("handover failed, this process keeps serving: %v", err)
				continue
			}
			handedOver.Store(true)
			srv.SetKeepAlivesEnabled(false)
			_ = mainLn.Close()
			log.Printf("handover complete: pid %d serves new requests", successor.Pid)
			break wait
		}
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
		s.Close()
		close(stopped)
	}()
	displayAddr := srv.Addr
	if strings.HasPrefix(displayAddr, ":") {
		displayAddr = "localhost" + displayAddr
	}
	serve := func() error { return srv.Serve(mainLn) }
	if *tlsCert != "" && *tlsKey != "" {
		log.Printf("MuseForge api listening on https://%s", displayAddr)
		serve = func() error { return srv.ServeTLS(mainLn, *tlsCert, *tlsKey) }
	} else {
		log.Printf("MuseForge api listening on http://%s", displayAddr)
	}
	signalReady(readyStatus)
	if err := serve(); err != nil && err != http.ErrServerClosed && !handedOver.Load() {
		log.Fatal(err)
	}
	<-stopped
}
