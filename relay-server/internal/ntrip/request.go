// Package ntrip implements minimal NTRIP v1/v2 request and response
// framing. The relay does not interpret RTCM/GGA payloads, only the
// HTTP-like header exchange used to establish a session.
package ntrip

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"strings"
)

// Request is a parsed NTRIP GET request line plus headers.
type Request struct {
	Mountpoint string
	Headers    map[string]string
}

// BasicAuth extracts the username/password from an Authorization: Basic header.
func (r *Request) BasicAuth() (username, password string, ok bool) {
	v, present := r.Headers["authorization"]
	if !present {
		return "", "", false
	}
	const prefix = "Basic "
	if !strings.HasPrefix(v, prefix) {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, prefix))
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// ReadRequest reads a single NTRIP GET request (request line + headers,
// terminated by a blank line) from r.
func ReadRequest(r *bufio.Reader) (*Request, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read request line: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "GET" {
		return nil, fmt.Errorf("unsupported request line: %q", line)
	}
	mountpoint := strings.TrimPrefix(fields[1], "/")

	headers := make(map[string]string)
	for {
		hline, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read headers: %w", err)
		}
		hline = strings.TrimRight(hline, "\r\n")
		if hline == "" {
			break
		}
		k, v, found := strings.Cut(hline, ":")
		if !found {
			continue
		}
		headers[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}

	return &Request{Mountpoint: mountpoint, Headers: headers}, nil
}

// WriteRequest writes an NTRIP v2 GET request for connecting to an
// upstream Caster (used when the relay itself acts as an NTRIP client
// toward the RTK provider, or vehiclesim toward the relay).
func WriteRequest(w *bufio.Writer, mountpoint, username, password string) error {
	return WriteRequestWithOptions(w, mountpoint, username, password, RequestOptions{})
}

// RequestOptions carries the provider-specific NTRIP request details
// that go beyond mountpoint/username/password - real RTK providers vary
// in NTRIP version and sometimes require extra headers
// (see rtk_provider_comparison.md and providerconfig.Provider).
type RequestOptions struct {
	// Version is "1" or "2"; empty defaults to "2". NTRIP v1 Casters
	// don't expect (and may reject) the Ntrip-Version header.
	Version string
	// ExtraHeaders are sent verbatim after the standard headers, for
	// whatever provider-specific parameter a given provider requires
	// (e.g. an account/contract ID header).
	ExtraHeaders map[string]string
}

// WriteRequestWithOptions is WriteRequest with provider-specific
// options applied.
func WriteRequestWithOptions(w *bufio.Writer, mountpoint, username, password string, opts RequestOptions) error {
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	var b strings.Builder
	fmt.Fprintf(&b, "GET /%s HTTP/1.1\r\n", mountpoint)
	b.WriteString("Host: ntrip-relay\r\n")
	if opts.Version != "1" {
		b.WriteString("Ntrip-Version: Ntrip/2.0\r\n")
	}
	b.WriteString("User-Agent: NTRIP fpv-japan-relay/0.1\r\n")
	fmt.Fprintf(&b, "Authorization: Basic %s\r\n", auth)
	for k, v := range opts.ExtraHeaders {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	b.WriteString("Connection: close\r\n\r\n")

	if _, err := w.WriteString(b.String()); err != nil {
		return err
	}
	return w.Flush()
}
