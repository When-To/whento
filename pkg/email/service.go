// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package email

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// Service handles email sending via SMTP
type Service struct {
	host        string
	port        int
	username    string
	password    string
	fromAddress string
	fromName    string
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
}

// NewService creates a new email service
func NewService(cfg Config, logger *slog.Logger) *Service {
	return &Service{
		host:        cfg.Host,
		port:        cfg.Port,
		username:    cfg.Username,
		password:    cfg.Password,
		fromAddress: cfg.FromAddress,
		fromName:    cfg.FromName,
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

// Send sends an email via SMTP
func (s *Service) Send(email Email) error {
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

	// Connect to SMTP server
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	// Setup authentication
	var auth smtp.Auth
	if s.username != "" && s.password != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	// Try to send with TLS first (port 465 or explicit STARTTLS)
	err := s.sendWithTLS(addr, auth, s.fromAddress, email.To, message)
	if err != nil {
		s.logger.Error("Failed to send email",
			slog.String("error", err.Error()),
			slog.String("to", to),
		)
		return fmt.Errorf("failed to send email: %w", err)
	}

	s.logger.Info("Email sent successfully",
		slog.String("to", to),
		slog.String("subject", email.Subject),
	)

	return nil
}

// sendWithTLS attempts to send email with TLS/STARTTLS
func (s *Service) sendWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	// For port 465 (implicit TLS)
	if s.port == 465 {
		// Create TLS config
		tlsConfig := &tls.Config{
			ServerName: s.host,
		}

		// Connect with TLS
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return err
		}
		defer conn.Close()

		// Create SMTP client
		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return err
		}
		defer client.Quit()

		// Authenticate
		if auth != nil {
			if err = client.Auth(auth); err != nil {
				return err
			}
		}

		// Set sender
		if err = client.Mail(from); err != nil {
			return err
		}

		// Set recipients
		for _, recipient := range to {
			if err = client.Rcpt(recipient); err != nil {
				return err
			}
		}

		// Send message
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write(msg)
		if err != nil {
			return err
		}
		err = w.Close()
		if err != nil {
			return err
		}

		return nil
	}

	// For port 587 or others (STARTTLS)
	return smtp.SendMail(addr, auth, from, to, msg)
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
