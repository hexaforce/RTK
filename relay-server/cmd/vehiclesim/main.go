// vehiclesim stands in for a car's NTRIP Client during local testing: it
// connects to the relay server, authenticates as one vehicle, sends a
// fake NMEA-GGA line every second, and logs how many RTCM bytes come
// back.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rtk-micros-dev/rtk-relay/internal/ntrip"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	relayAddr := envOr("RELAY_ADDR", "localhost:2101")
	vehicleID := envOr("VEHICLE_ID", "vehicle-001")
	apiKey := envOr("VEHICLE_API_KEY", "dev-key-001")
	mountpoint := envOr("MOUNTPOINT", "RELAY")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := net.Dial("tcp", relayAddr)
	if err != nil {
		logger.Error("dial relay failed", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	if err := ntrip.WriteRequest(w, mountpoint, vehicleID, apiKey); err != nil {
		logger.Error("write request failed", "err", err)
		os.Exit(1)
	}

	ok, status, err := ntrip.ReadResponseStatus(r)
	if err != nil {
		logger.Error("read response failed", "err", err)
		os.Exit(1)
	}
	if !ok {
		logger.Error("relay rejected session", "status", status)
		os.Exit(1)
	}
	logger.Info("session established with relay", "relay_addr", relayAddr, "vehicle_id", vehicleID)

	go readRTCM(ctx, r, logger)
	sendGGA(ctx, w, logger)
}

func sendGGA(ctx context.Context, w *bufio.Writer, logger *slog.Logger) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			gga := fmt.Sprintf(
				"$GPGGA,%s,3541.4083,N,13945.6534,E,1,08,0.9,10.0,M,36.5,M,,*6E\r\n",
				t.UTC().Format("150405.00"),
			)
			if _, err := w.WriteString(gga); err != nil {
				logger.Error("write GGA failed", "err", err)
				return
			}
			if err := w.Flush(); err != nil {
				logger.Error("flush GGA failed", "err", err)
				return
			}
		}
	}
}

func readRTCM(ctx context.Context, r *bufio.Reader, logger *slog.Logger) {
	buf := make([]byte, 4096)
	var total int64
	for ctx.Err() == nil {
		n, err := r.Read(buf)
		if n > 0 {
			total += int64(n)
			logger.Info("received RTCM bytes", "n", n, "total", total)
		}
		if err != nil {
			logger.Info("rtcm stream ended", "err", err, "total", total)
			return
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
