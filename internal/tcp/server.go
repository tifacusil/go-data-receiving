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

// Server listens for DTU connections and dispatches frames.
type Server struct {
	port   int
	parser Parser
	sink   Sink
	dlx    DeadLetter
	status *Status
	logger *slog.Logger
}

// NewServer wires a Server. status may be nil (no health endpoint yet).
func NewServer(port int, parser Parser, sink Sink, dlx DeadLetter, status *Status, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{port: port, parser: parser, sink: sink, dlx: dlx, status: status, logger: logger}
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

		result := s.parser.Parse(frame)
		if result.Success && result.Envelope != nil {
			if err := s.sink.Write(ctx, result.Envelope); err != nil {
				s.logger.ErrorContext(ctx, "sink write failed", slog.Any("error", err),
					slog.String("message_id", result.Envelope.MessageID))
				// Mirror C#: an ACK is still sent below only on success path; a
				// sink failure aborts this frame without a success ACK.
				return
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
			s.writeAck(ctx, conn, ackStatus(perr.Stage), byte(perr.Stage))
		}
	}
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
