package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fightheyyy/barena/platform/internal/control"
)

func main() {
	if err := run(); err != nil {
		slog.Error("barena-server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	address := env("BARENA_SERVER_ADDR", "127.0.0.1:8787")
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("BARENA_SERVER_ADDR must use an IP address")
	}
	redirectURL := os.Getenv("BARENA_GITHUB_REDIRECT_URL")
	githubClientID := os.Getenv("BARENA_GITHUB_CLIENT_ID")
	githubClientSecret := os.Getenv("BARENA_GITHUB_CLIENT_SECRET")
	gatewaySecret := os.Getenv("BARENA_GATEWAY_SECRET")
	if err := validateBinding(
		ip,
		os.Getenv("BARENA_ALLOW_REMOTE") == "1",
		os.Getenv("BARENA_INTERNAL_MODE") == "1",
		gatewaySecret,
		githubClientID,
		githubClientSecret,
		redirectURL,
	); err != nil {
		return err
	}
	databaseURL := os.Getenv("BARENA_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("BARENA_DATABASE_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := control.OpenPostgres(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if interrupted, err := store.InterruptActiveRuns(ctx); err != nil {
		return err
	} else if interrupted > 0 {
		slog.Warn("marked stale Runs interrupted", "count", interrupted)
	}

	repoRoot, err := filepath.Abs(env("BARENA_REPO_ROOT", ".."))
	if err != nil {
		return err
	}
	runner, err := control.NewRunnerManager(store, control.RunnerConfig{
		NodeCommand: env("BARENA_NODE", "node"),
		WorkerPath:  env("BARENA_ENGINE_WORKER", filepath.Join(repoRoot, "dist", "engine-worker.js")),
		RunsRoot:    env("BARENA_RUNS_ROOT", filepath.Join(repoRoot, "runs")),
		KillGrace:   5 * time.Second,
	})
	if err != nil {
		return err
	}
	evolutionRuntime, err := control.NewEvolutionRuntimeManager(control.EvolutionRuntimeConfig{
		NodeCommand:   env("BARENA_NODE", "node"),
		WorkerPath:    env("BARENA_XIAOBA_EVOLUTION_WORKER", filepath.Join(repoRoot, "dist", "evolution-runtime-worker.js")),
		XiaoBaCommand: env("BARENA_XIAOBA_COMMAND", "xiaoba"),
		ProjectRoot:   os.Getenv("BARENA_XIAOBA_PROJECT_ROOT"),
		RolesRoot:     os.Getenv("BARENA_XIAOBA_ROLES_ROOT"),
		SkillsRoot:    os.Getenv("BARENA_XIAOBA_SKILLS_ROOT"),
		WorkspaceRoot: env("BARENA_XIAOBA_EVOLUTION_ROOT", filepath.Join(repoRoot, "runs", "cloud-evolution")),
		EnvAllowlist:  splitList(os.Getenv("BARENA_XIAOBA_ENV_ALLOWLIST")),
		ProbeTimeout:  8 * time.Second,
		CacheTTL:      5 * time.Second,
	})
	if err != nil {
		return err
	}
	handler, err := control.NewHTTPHandlerWithRuntime(store, runner, control.AuthConfig{
		GitHubClientID:     githubClientID,
		GitHubClientSecret: githubClientSecret,
		RedirectURL:        redirectURL,
		SecureCookies:      strings.HasPrefix(redirectURL, "https://"),
		GatewaySecret:      gatewaySecret,
	}, evolutionRuntime)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	slog.Info("barena-server listening", "address", address)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func validateBinding(
	ip net.IP,
	allowRemote bool,
	internalMode bool,
	gatewaySecret string,
	githubClientID string,
	githubClientSecret string,
	redirectURL string,
) error {
	if ip.IsLoopback() {
		return nil
	}
	if !allowRemote {
		return errors.New("remote binding requires BARENA_ALLOW_REMOTE=1")
	}
	if internalMode {
		if len(gatewaySecret) < 32 {
			return errors.New("internal remote binding requires a 32+ character BARENA_GATEWAY_SECRET")
		}
		return nil
	}
	if githubClientID == "" || githubClientSecret == "" ||
		!strings.HasPrefix(redirectURL, "https://") {
		return errors.New(
			"remote binding requires GitHub OAuth and an HTTPS redirect URL",
		)
	}
	return nil
}
