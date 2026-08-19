package ntrip

import (
	"bufio"
	"fmt"
	"strings"
)

// WriteOK writes the NTRIP v1-style success response that precedes the
// RTCM stream. Most NTRIP clients only look for the leading "ICY 200 OK".
func WriteOK(w *bufio.Writer) error {
	if _, err := w.WriteString("ICY 200 OK\r\n\r\n"); err != nil {
		return err
	}
	return w.Flush()
}

// WriteError writes an NTRIP/HTTP error response with the given status
// line (e.g. "401 Unauthorized").
func WriteError(w *bufio.Writer, status string) error {
	body := fmt.Sprintf("ERROR - %s\r\n", status)
	resp := fmt.Sprintf(
		"HTTP/1.1 %s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		status, len(body), body,
	)
	if _, err := w.WriteString(resp); err != nil {
		return err
	}
	return w.Flush()
}

// ReadResponseStatus reads a single status line from an upstream Caster
// (either "ICY 200 OK" or a standard "HTTP/1.x 200 OK") and reports
// whether the session was accepted.
func ReadResponseStatus(r *bufio.Reader) (ok bool, status string, err error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return false, "", fmt.Errorf("read status line: %w", err)
	}
	status = strings.TrimRight(line, "\r\n")
	ok = strings.Contains(status, "200")
	if !ok {
		return false, status, nil
	}
	// NTRIP v2 upstreams send full HTTP headers after the status line;
	// v1-style Casters send "ICY 200 OK\r\n\r\n" with no further headers.
	if strings.HasPrefix(status, "HTTP/") {
		for {
			hline, err := r.ReadString('\n')
			if err != nil {
				return false, status, fmt.Errorf("read upstream headers: %w", err)
			}
			if strings.TrimRight(hline, "\r\n") == "" {
				break
			}
		}
	} else {
		// Consume the blank line following "ICY 200 OK".
		if _, err := r.ReadString('\n'); err != nil {
			return false, status, fmt.Errorf("read upstream trailer: %w", err)
		}
	}
	return true, status, nil
}
