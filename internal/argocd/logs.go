package argocd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
)

// PodLogsOptions configures the parameters for fetching or streaming pod logs from Argo CD.
type PodLogsOptions struct {
	PodName    string
	Container  string
	Follow     bool
	TailLines  int64
	Previous   bool
	Timestamps bool
}

// restLogStreamEnvelope matches the JSON-lines envelope returned by the Argo CD REST PodLogs endpoint.
type restLogStreamEnvelope struct {
	Result struct {
		Content      string `json:"content"`
		TimeStampStr string `json:"timeStampStr"`
		Last         bool   `json:"last"`
	} `json:"result"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// StreamPodLogs connects to the Argo CD PodLogs endpoint and writes log lines to out.
// It uses the gRPC ApplicationService.PodLogs stream first and falls back to REST streaming if needed.
func (c *Client) StreamPodLogs(ctx context.Context, resolved *ResolvedApp, opts PodLogsOptions, out io.Writer) error {
	if resolved == nil || resolved.AppName == "" {
		return fmt.Errorf("resolved Argo CD application cannot be empty")
	}
	if strings.TrimSpace(opts.PodName) == "" {
		return fmt.Errorf("pod name cannot be empty")
	}

	err := c.streamPodLogsGRPC(ctx, resolved, opts, out)
	if err == nil {
		return nil
	}

	// Fallback to the REST streaming endpoint if gRPC stream initialization fails.
	if restErr := c.streamPodLogsREST(ctx, resolved, opts, out); restErr == nil {
		return nil
	}

	return fmt.Errorf("failed to stream logs for pod %q in application %q: %w", opts.PodName, resolved.AppName, err)
}

func (c *Client) streamPodLogsGRPC(ctx context.Context, resolved *ResolvedApp, opts PodLogsOptions, out io.Writer) error {
	conn, appClient, err := c.NewApplicationClient()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	appName := resolved.AppName
	targetNS := resolved.TargetNamespace
	podName := opts.PodName
	follow := opts.Follow
	previous := opts.Previous

	query := &application.ApplicationPodLogsQuery{
		Name:      &appName,
		Namespace: &targetNS,
		PodName:   &podName,
		Follow:    &follow,
		Previous:  &previous,
	}
	if resolved.AppNamespace != "" {
		appNS := resolved.AppNamespace
		query.AppNamespace = &appNS
	}
	if opts.Container != "" {
		container := opts.Container
		query.Container = &container
	}
	if opts.TailLines >= 0 {
		tail := opts.TailLines
		query.TailLines = &tail
	}

	stream, err := appClient.PodLogs(ctx, query)
	if err != nil {
		return err
	}

	for {
		entry, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			return nil
		}
		if recvErr != nil {
			// Treat context cancellation (Ctrl+C during -f) as a clean exit.
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return recvErr
		}
		if entry == nil {
			continue
		}

		line := entry.GetContent()
		if opts.Timestamps && entry.GetTimeStampStr() != "" && !strings.HasPrefix(line, entry.GetTimeStampStr()) {
			line = entry.GetTimeStampStr() + " " + line
		}
		if _, writeErr := fmt.Fprintln(out, line); writeErr != nil {
			return writeErr
		}
		if entry.GetLast() && !opts.Follow {
			return nil
		}
	}
}

func (c *Client) streamPodLogsREST(ctx context.Context, resolved *ResolvedApp, opts PodLogsOptions, out io.Writer) error {
	apiPath := fmt.Sprintf(
		"/api/v1/applications/%s/pods/%s/logs",
		url.PathEscape(resolved.AppName),
		url.PathEscape(opts.PodName),
	)
	endpoint, err := c.buildRESTURL(apiPath, resolved.AppNamespace)
	if err != nil {
		return err
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	q := parsed.Query()
	q.Set("namespace", resolved.TargetNamespace)
	q.Set("follow", strconv.FormatBool(opts.Follow))
	q.Set("previous", strconv.FormatBool(opts.Previous))
	q.Set("timestamps", strconv.FormatBool(opts.Timestamps))
	if opts.Container != "" {
		q.Set("container", opts.Container)
	}
	if opts.TailLines >= 0 {
		q.Set("tailLines", strconv.FormatInt(opts.TailLines, 10))
	}
	parsed.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AuthToken)
	req.Header.Set("Accept", "application/json")

	// Use timeout=0 so long-lived `-f` log streams do not time out prematurely.
	httpClient := c.newHTTPClient(0)
	resp, err := httpClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GET %s returned HTTP %d: %s", parsed.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Allow up to 1MB per log line.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}

		var env restLogStreamEnvelope
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			// If response is plain text rather than JSON-lines, write it directly.
			if _, writeErr := fmt.Fprintln(out, raw); writeErr != nil {
				return writeErr
			}
			continue
		}

		if env.Error.Message != "" {
			return errors.New(env.Error.Message)
		}

		line := env.Result.Content
		if opts.Timestamps && env.Result.TimeStampStr != "" && !strings.HasPrefix(line, env.Result.TimeStampStr) {
			line = env.Result.TimeStampStr + " " + line
		}
		if _, writeErr := fmt.Fprintln(out, line); writeErr != nil {
			return writeErr
		}
		if env.Result.Last && !opts.Follow {
			return nil
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(ctx.Err(), context.Canceled) {
		return err
	}
	return nil
}
