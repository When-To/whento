// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	// Aliased: NewService takes a *slog.Logger named `logger`, which would
	// otherwise shadow the package.
	pkglog "github.com/whento/pkg/logger"
)

const (
	// defaultDialTimeout bounds the TCP connect and, on port 465, the TLS handshake.
	// Without it a host that accepts the connection and then says nothing holds the
	// caller for as long as the operating system's own connect timeout — minutes — and
	// the callers here are detached goroutines nobody is waiting on, so they simply
	// accumulate, one per notification.
	defaultDialTimeout = 10 * time.Second

	// defaultTimeout bounds the whole SMTP conversation, greeting included. Reaching a
	// host is not the same as it answering: a server that completes the TCP handshake
	// and never sends its 220 banner would otherwise park the goroutine for ever.
	defaultTimeout = 30 * time.Second

	// implicitTLSPort speaks TLS from the first byte, rather than upgrading with
	// STARTTLS the way 587 does.
	implicitTLSPort = 465
)

// Service handles email sending via SMTP
type Service struct {
	host        string
	port        int
	username    string
	password    string
	fromAddress string
	fromName    string
	dialTimeout time.Duration
	timeout     time.Duration
	logger      *slog.Logger
}

// Config holds email service configuration
type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string

	// DialTimeout bounds the connection attempt. Zero means defaultDialTimeout.
	DialTimeout time.Duration
	// Timeout bounds the whole conversation. Zero means defaultTimeout.
	Timeout time.Duration
}

// NewService creates a new email service
func NewService(cfg Config, logger *slog.Logger) *Service {
	dialTimeout := cfg.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return &Service{
		host:        cfg.Host,
		port:        cfg.Port,
		username:    cfg.Username,
		password:    cfg.Password,
		fromAddress: cfg.FromAddress,
		fromName:    cfg.FromName,
		dialTimeout: dialTimeout,
		timeout:     timeout,
		logger:      logger,
	}
}

// Email represents an email message
type Email struct {
	To      []string
	Subject string
	Body    string
	HTML    bool
}

// Send sends an email via SMTP, bounded by the service's own timeout.
func (s *Service) Send(email Email) error {
	return s.SendContext(context.Background(), email)
}

// SendContext sends an email via SMTP, giving up when ctx does.
//
// A caller with a deadline of its own keeps it; anything else is bounded by the service
// timeout, so no send can outlive it.
func (s *Service) SendContext(ctx context.Context, email Email) error {
	// Validate configuration
	if s.host == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	// Build message
	from := s.buildFromHeader()
	to := strings.Join(email.To, ", ")

	var contentType string
	if email.HTML {
		contentType = "text/html; charset=UTF-8"
	} else {
		contentType = "text/plain; charset=UTF-8"
	}

	message := []byte(
		"From: " + from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + email.Subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: " + contentType + "\r\n" +
			"\r\n" +
			email.Body + "\r\n",
	)

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	// Connect to SMTP server
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))

	// Setup authentication
	var auth smtp.Auth
	if s.username != "" && s.password != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	// Try to send with TLS first (port 465 or explicit STARTTLS)
	// Neither line below carries a recipient address. This service is the one
	// place every outbound mail passes through, so logging `to` here would put an
	// address in the log stream for every verification, magic link, reset and
	// notification the instance ever sends — the largest single source of them.
	// The fingerprint still groups retries of the same send, and the recipient
	// count still says whether a failure hit one person or a batch.
	recipients := pkglog.Fingerprint(to)

	err := s.sendWithTLS(ctx, addr, auth, s.fromAddress, email.To, message)
	if err != nil {
		s.logger.Error("Failed to send email",
			slog.String("error", err.Error()),
			slog.String("recipient_ref", recipients),
			slog.Int("recipient_count", len(email.To)),
		)
		return fmt.Errorf("failed to send email: %w", err)
	}

	s.logger.Info("Email sent successfully",
		slog.String("recipient_ref", recipients),
		slog.Int("recipient_count", len(email.To)),
		slog.String("subject", email.Subject),
	)

	return nil
}

// sendWithTLS attempts to send email with TLS/STARTTLS
func (s *Service) sendWithTLS(ctx context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := s.dial(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// One deadline for the whole conversation. net/smtp has no context of its own, so
	// this is what keeps a silent server from parking the goroutine on a read.
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}

	// Create SMTP client
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	// For anything but implicit TLS, upgrade the plain connection when the server
	// offers it — this is what port 587 expects.
	if s.port != implicitTLSPort {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
				return err
			}
		}
	}

	// Authenticate
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp: server does not support AUTH")
		}
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	// Set sender
	if err := client.Mail(from); err != nil {
		return err
	}

	// Set recipients
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}

	// Send message
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return client.Quit()
}

// dial opens the connection, in TLS from the first byte on port 465 and in the clear
// elsewhere. Both paths carry a timeout: the dialer's, and the context's on top of it.
func (s *Service) dial(ctx context.Context, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: s.dialTimeout}

	if s.port == implicitTLSPort {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config:    &tls.Config{ServerName: s.host},
		}

		return tlsDialer.DialContext(ctx, "tcp", addr)
	}

	return dialer.DialContext(ctx, "tcp", addr)
}

// buildFromHeader builds the From header with optional name
func (s *Service) buildFromHeader() string {
	if s.fromName != "" {
		return fmt.Sprintf("%s <%s>", s.fromName, s.fromAddress)
	}
	return s.fromAddress
}

// IsConfigured returns true if SMTP is configured
func (s *Service) IsConfigured() bool {
	return s.host != ""
}
