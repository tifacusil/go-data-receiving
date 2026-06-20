// Package tcp implements the DTU frame server: it accepts TCP connections, reads
// length-prefixed frames, runs them through the parser, writes successes to the
// sink and rejects to the dead-letter writer, and replies with an ACK.
// Port of the C# TcpServer + TcpServerHostedService.
package tcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

// Parser turns a raw frame into a ParseResult. Satisfied by *parsing.Parser.
type Parser interface {
	Parse(rawFrame []byte) parsing.ParseResult
}

// Sink consumes a parsed envelope. Satisfied by the sink package.
type Sink interface {
	Write(ctx context.Context, env *parsing.Envelope) error
}

// DeadLetter writes a rejected frame. Satisfied by the deadletter package.
type DeadLetter interface {
	Write(ctx context.Context, rawFrame []byte, perr *parsing.ParseError) error
}

// Observer records per-frame metrics. Satisfied by *observability.Metrics. It is
// optional (nil = no metrics) so the server stays usable without observability.
type Observer interface {
	ObserveFrame(result, sensorType string, seconds float64)
	IncPublished()
	IncDropped()
}

// Tracer opens a span per frame. Satisfied by *observability.Tracer. Optional.
type Tracer interface {
	StartFrame(ctx context.Context, sensorType, messageID string) (context.Context, func())
}

// Server listens for DTU connections and dispatches frames.
type Server struct {
	port    int
	parser  Parser
	sink    Sink
	dlx     DeadLetter
	status  *Status
	logger  *slog.Logger
	metrics Observer
	tracer  Tracer
}

// NewServer wires a Server. status may be nil (no health endpoint yet).
func NewServer(port int, parser Parser, sink Sink, dlx DeadLetter, status *Status, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{port: port, parser: parser, sink: sink, dlx: dlx, status: status, logger: logger}
}

// WithObservability attaches optional metrics and tracing. Either may be nil.
func (s *Server) WithObservability(m Observer, t Tracer) *Server {
	s.metrics = m
	s.tracer = t
	return s
}

// Run binds the listener and serves until ctx is cancelled. It blocks; run it in
// a goroutine (errgroup) from the composition root.
func (s *Server) Run(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("tcp listen on :%d: %w", s.port, err)
	}
	return s.serve(ctx, ln)
}

// serve runs the accept loop on an existing listener. Exposed for tests that
// bind an ephemeral port.
func (s *Server) serve(ctx context.Context, ln net.Listener) error {
	s.logger.InfoContext(ctx, "tcp server listening", slog.String("addr", ln.Addr().String()))
	if s.status != nil {
		s.status.SetListening(true)
	}

	// Cancellation unblocks Accept by closing the listener.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	var wg sync.WaitGroup
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break // shutting down
			}
			s.logger.ErrorContext(ctx, "accept failed", slog.Any("error", err))
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleConn(ctx, conn)
		}()
	}

	wg.Wait()
	if s.status != nil {
		s.status.SetListening(false)
	}
	s.logger.InfoContext(ctx, "tcp server stopped")
	return nil
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	for {
		if ctx.Err() != nil {
			return
		}
		frame, err := readFrame(conn)
		if err != nil {
			if errors.Is(err, errClosed) || errors.Is(err, io.EOF) {
				return // clean close
			}
			if ctx.Err() == nil {
				s.logger.ErrorContext(ctx, "error reading frame", slog.Any("error", err))
			}
			return
		}

		start := time.Now()
		result := s.parser.Parse(frame)
		if result.Success && result.Envelope != nil {
			env := result.Envelope
			frameCtx, endSpan := s.startSpan(ctx, env.SensorType, env.MessageID)
			err := s.sink.Write(frameCtx, env)
			endSpan()
			if err != nil {
				s.logger.ErrorContext(ctx, "sink write failed", slog.Any("error", err),
					slog.String("message_id", env.MessageID))
				// Mirror C#: an ACK is still sent below only on success path; a
				// sink failure aborts this frame without a success ACK.
				return
			}
			s.observe("ok", env.SensorType, start)
			if s.metrics != nil {
				s.metrics.IncPublished()
			}
			s.writeAck(ctx, conn, 0x00, 0x00)
			continue
		}

		if result.Error != nil {
			perr := result.Error
			s.logger.WarnContext(ctx, "frame rejected",
				slog.String("stage", perr.Stage.String()),
				slog.String("error", perr.Message),
				slog.String("raw", hexPreview(frame)))
			if err := s.dlx.Write(ctx, frame, perr); err != nil {
				s.logger.ErrorContext(ctx, "dlx write failed", slog.Any("error", err))
			}
			s.observe("rejected", "unknown", start)
			if s.metrics != nil {
				s.metrics.IncDropped()
			}
			s.writeAck(ctx, conn, ackStatus(perr.Stage), byte(perr.Stage))
		}
	}
}

// observe records frame latency + result. No-op when metrics are not attached.
func (s *Server) observe(result, sensorType string, start time.Time) {
	if s.metrics != nil {
		s.metrics.ObserveFrame(result, sensorType, time.Since(start).Seconds())
	}
}

// startSpan opens a tracing span when a tracer is attached, else returns ctx
// unchanged with a no-op end func.
func (s *Server) startSpan(ctx context.Context, sensorType, messageID string) (context.Context, func()) {
	if s.tracer == nil {
		return ctx, func() {}
	}
	return s.tracer.StartFrame(ctx, sensorType, messageID)
}

func (s *Server) writeAck(ctx context.Context, conn net.Conn, status, code byte) {
	if _, err := conn.Write(buildAck(status, code)); err != nil {
		s.logger.ErrorContext(ctx, "ack write failed", slog.Any("error", err))
	}
}

func hexPreview(frame []byte) string {
	n := len(frame)
	if n > 16 {
		n = 16
	}
	const digits = "0123456789ABCDEF"
	out := make([]byte, 0, n*2)
	for _, b := range frame[:n] {
		out = append(out, digits[b>>4], digits[b&0x0F])
	}
	return string(out)
}
