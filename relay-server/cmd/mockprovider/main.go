// mockprovider stands in for a real RTK provider's NTRIP Caster during
// local development, since no provider has been selected yet (see
// rtk_provider_comparison.md). It accepts one NTRIP GET request per
// connection, checks Basic auth against MOCK_PROVIDER_USERNAME/PASSWORD,
// logs any GGA lines it receives, and streams fake RTCM-shaped bytes
// back at a fixed interval.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rtk-micros-dev/rtk-relay/internal/ntrip"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	listenAddr := envOr("LISTEN_ADDR", ":2201")
	mountpoint := envOr("MOCK_PROVIDER_MOUNTPOINT", "TESTMOUNT")
	username := envOr("MOCK_PROVIDER_USERNAME", "relay")
	password := envOr("MOCK_PROVIDER_PASSWORD", "relay-secret")
	// Simulates a provider that requires an extra connection parameter
	// beyond mountpoint/username/password (e.g. an account ID header),
	// to exercise providerconfig.Provider.ExtraHeaders end-to-end.
	// Unset (the default) means no extra header is required.
	requiredHeaderKey := envOr("MOCK_PROVIDER_REQUIRE_HEADER_KEY", "")
	requiredHeaderValue := envOr("MOCK_PROVIDER_REQUIRE_HEADER_VALUE", "")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.Error("listen failed", "err", err)
		os.Exit(1)
	}
	defer ln.Close()
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	logger.Info("mock provider listening", "addr", listenAddr, "mountpoint", mountpoint)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("accept failed", "err", err)
			continue
		}
		go handleConn(ctx, conn, logger, mountpoint, username, password, requiredHeaderKey, requiredHeaderValue)
	}
}

func handleConn(ctx context.Context, conn net.Conn, logger *slog.Logger, mountpoint, username, password, requiredHeaderKey, requiredHeaderValue string) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	req, err := ntrip.ReadRequest(r)
	if err != nil {
		logger.Warn("bad request", "err", err)
		return
	}
	logger.Info("request headers", "headers", req.Headers)

	if req.Mountpoint != mountpoint {
		logger.Warn("unknown mountpoint requested", "mountpoint", req.Mountpoint)
		_ = ntrip.WriteError(w, "404 Not Found")
		return
	}
	user, pass, ok := req.BasicAuth()
	if !ok || user != username || pass != password {
		logger.Warn("auth rejected", "user", user)
		_ = ntrip.WriteError(w, "401 Unauthorized")
		return
	}
	if requiredHeaderKey != "" {
		got, present := req.Headers[strings.ToLower(requiredHeaderKey)]
		if !present || got != requiredHeaderValue {
			logger.Warn("missing/mismatched required header", "key", requiredHeaderKey, "got", got, "present", present)
			_ = ntrip.WriteError(w, "400 Bad Request")
			return
		}
	}
	if err := ntrip.WriteOK(w); err != nil {
		return
	}
	logger.Info("session accepted", "remote", conn.RemoteAddr().String())

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go readGGA(connCtx, r, logger)
	writeFakeRTCM(connCtx, w, logger)
}

func readGGA(ctx context.Context, r *bufio.Reader, logger *slog.Logger) {
	for ctx.Err() == nil {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		logger.Info("received GGA", "line", line)
	}
}

// writeFakeRTCM writes a small binary chunk every second that looks
// enough like an RTCM3 frame (0xD3 preamble + length + payload) to
// exercise the relay's transparent byte-passthrough path.
func writeFakeRTCM(ctx context.Context, w *bufio.Writer, logger *slog.Logger) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	var seq uint32

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			payload := make([]byte, 16)
			if _, err := rand.Read(payload); err != nil {
				logger.Error("rand read failed", "err", err)
				return
			}
			binary.BigEndian.PutUint32(payload[:4], seq)
			seq++

			frame := make([]byte, 0, 3+len(payload)+3)
			frame = append(frame, 0xD3, byte(len(payload)>>8), byte(len(payload)))
			frame = append(frame, payload...)
			frame = append(frame, 0, 0, 0) // fake CRC24, not validated by the relay

			if _, err := w.Write(frame); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
