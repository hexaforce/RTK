// Package relay implements the core NTRIP relay: it terminates NTRIP
// sessions from vehicles (acting as an NTRIP Caster) and, once the
// vehicle is authenticated, opens a matching session to the upstream
// RTK provider (acting as an NTRIP Client), then splices GGA/RTCM bytes
// between the two connections without interpreting them.
//
// See rtk_relay_protocol_design.md for the full design this implements.
package relay

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/fpv-japan/rtk-relay/internal/auth"
	"github.com/fpv-japan/rtk-relay/internal/ntrip"
	"github.com/fpv-japan/rtk-relay/internal/providerconfig"
)

// Server accepts vehicle NTRIP connections and relays them to a single
// upstream provider.
type Server struct {
	ListenAddr    string
	Provider      providerconfig.Provider
	Auth          auth.VehicleAuthenticator
	DialTimeout   time.Duration
	Logger        *slog.Logger
}

func (s *Server) dialTimeout() time.Duration {
	if s.DialTimeout > 0 {
		return s.DialTimeout
	}
	return 10 * time.Second
}

// ListenAndServe accepts connections until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", s.ListenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	s.Logger.Info("relay server listening", "addr", s.ListenAddr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handleVehicleConn(ctx, conn)
	}
}

func (s *Server) handleVehicleConn(ctx context.Context, vconn net.Conn) {
	defer vconn.Close()
	remote := vconn.RemoteAddr().String()

	vr := bufio.NewReader(vconn)
	vw := bufio.NewWriter(vconn)

	req, err := ntrip.ReadRequest(vr)
	if err != nil {
		s.Logger.Warn("failed to read vehicle request", "remote", remote, "err", err)
		return
	}

	vehicleID, apiKey, ok := req.BasicAuth()
	if !ok {
		s.Logger.Warn("vehicle request missing basic auth", "remote", remote)
		_ = ntrip.WriteError(vw, "401 Unauthorized")
		return
	}

	authed, err := s.Auth.Authenticate(ctx, vehicleID, apiKey)
	if err != nil {
		s.Logger.Error("vehicle auth backend error", "remote", remote, "vehicle_id", vehicleID, "err", err)
		_ = ntrip.WriteError(vw, "502 Bad Gateway")
		return
	}
	if !authed {
		s.Logger.Warn("vehicle auth rejected", "remote", remote, "vehicle_id", vehicleID)
		_ = ntrip.WriteError(vw, "401 Unauthorized")
		return
	}

	pconn, err := net.DialTimeout("tcp", s.Provider.Addr(), s.dialTimeout())
	if err != nil {
		s.Logger.Error("provider dial failed", "vehicle_id", vehicleID, "provider_addr", s.Provider.Addr(), "err", err)
		_ = ntrip.WriteError(vw, "502 Bad Gateway")
		return
	}
	defer pconn.Close()

	pw := bufio.NewWriter(pconn)
	pr := bufio.NewReader(pconn)

	if err := ntrip.WriteRequest(pw, s.Provider.Mountpoint, s.Provider.Username, s.Provider.Password); err != nil {
		s.Logger.Error("provider request failed", "vehicle_id", vehicleID, "err", err)
		_ = ntrip.WriteError(vw, "502 Bad Gateway")
		return
	}

	accepted, status, err := ntrip.ReadResponseStatus(pr)
	if err != nil {
		s.Logger.Error("provider response read failed", "vehicle_id", vehicleID, "err", err)
		_ = ntrip.WriteError(vw, "502 Bad Gateway")
		return
	}
	if !accepted {
		s.Logger.Warn("provider rejected session", "vehicle_id", vehicleID, "status", status)
		_ = ntrip.WriteError(vw, "502 Bad Gateway")
		return
	}

	if err := ntrip.WriteOK(vw); err != nil {
		s.Logger.Warn("failed to ack vehicle", "vehicle_id", vehicleID, "err", err)
		return
	}

	s.Logger.Info("session established", "vehicle_id", vehicleID, "remote", remote, "mountpoint", s.Provider.Mountpoint)
	start := time.Now()
	ggaBytes, rtcmBytes := s.splice(vconn, vr, pconn, pr)
	s.Logger.Info("session closed",
		"vehicle_id", vehicleID,
		"duration_sec", time.Since(start).Seconds(),
		"gga_bytes", ggaBytes,
		"rtcm_bytes", rtcmBytes,
	)
}

// splice copies GGA bytes vehicle->provider and RTCM bytes
// provider->vehicle concurrently, without interpreting either stream,
// until one side closes or errors.
func (s *Server) splice(vconn net.Conn, vr *bufio.Reader, pconn net.Conn, pr *bufio.Reader) (ggaBytes, rtcmBytes int64) {
	done := make(chan struct{}, 2)

	go func() {
		n, err := io.Copy(pconn, vr)
		ggaBytes = n
		if err != nil && !isClosedErr(err) {
			s.Logger.Debug("gga copy ended", "err", err)
		}
		if c, ok := pconn.(interface{ CloseWrite() error }); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}()

	go func() {
		n, err := io.Copy(vconn, pr)
		rtcmBytes = n
		if err != nil && !isClosedErr(err) {
			s.Logger.Debug("rtcm copy ended", "err", err)
		}
		if c, ok := vconn.(interface{ CloseWrite() error }); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}()

	<-done
	vconn.Close()
	pconn.Close()
	<-done
	return ggaBytes, rtcmBytes
}

func isClosedErr(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF)
}
