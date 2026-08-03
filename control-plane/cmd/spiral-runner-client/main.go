// spiral-runner-client preserves the existing Go control-plane child-process
// contract while moving execution ownership to the internal spiral-runner
// service. It is the Compose transport adapter, not a second evaluation path.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const maxRequestBytes = 2 * 1024 * 1024

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return errors.New("spiral-runner-client expects exactly one Worker marker argument")
	}
	endpoint, err := workerEndpoint(filepath.Base(os.Args[1]))
	if err != nil {
		return err
	}
	base, err := runnerURL(os.Getenv("SPIRAL_RUNNER_URL"))
	if err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(os.Stdin, maxRequestBytes+1))
	if err != nil {
		return err
	}
	if len(body) == 0 || len(body) > maxRequestBytes {
		return errors.New("Worker request must contain from 1 to 2097152 bytes")
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(base.String(), "/")+endpoint,
		strings.NewReader(string(body)),
	)
	if err != nil {
		return err
	}
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("spiral-runner request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return fmt.Errorf(
			"spiral-runner returned %s: %s",
			response.Status,
			strings.TrimSpace(string(detail)),
		)
	}
	if _, err := io.Copy(os.Stdout, response.Body); err != nil {
		return fmt.Errorf("spiral-runner response failed: %w", err)
	}
	if endpoint == "/v1/engine/run" && response.Trailer.Get("X-Spiral-Runner-Status") != "ok" {
		detail := "remote Engine Worker failed"
		if encoded := response.Trailer.Get("X-Spiral-Runner-Error"); encoded != "" {
			if decoded, decodeErr := base64.RawURLEncoding.DecodeString(encoded); decodeErr == nil {
				detail = string(decoded)
			}
		}
		return errors.New(detail)
	}
	return nil
}

func workerEndpoint(marker string) (string, error) {
	switch marker {
	case "engine-worker.js":
		return "/v1/engine/run", nil
	case "evolution-runtime-worker.js":
		return "/v1/evolution/run", nil
	default:
		return "", fmt.Errorf("unsupported Worker marker: %s", marker)
	}
}

func runnerURL(value string) (*url.URL, error) {
	if value == "" {
		return nil, errors.New("SPIRAL_RUNNER_URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("SPIRAL_RUNNER_URL must be an HTTP(S) origin")
	}
	return parsed, nil
}
