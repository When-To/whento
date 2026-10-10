// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package email

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSendContextCancellationInterruptsSilentSMTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		close(accepted)
		_, _ = io.Copy(io.Discard, conn)
	}()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("SMTP listener did not return a TCP address")
	}
	s := NewService(Config{Host: "127.0.0.1", Port: addr.Port, FromAddress: "no-reply@example.test"}, discardLogger())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.SendContext(ctx, Email{To: []string{"ada@example.test"}, Subject: "Reminder"}) }()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("SMTP connection was not established")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled SMTP send succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled send waited for the 30-second socket deadline")
	}
}

func TestSMTPRejectionCannotLogServerEchoedAddressOrSecret(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = fmt.Fprint(conn, "220 test SMTP\r\n")
		r := bufio.NewScanner(conn)
		for r.Scan() {
			if strings.HasPrefix(r.Text(), "RCPT") {
				_, _ = fmt.Fprint(conn, "550 ada@example.test rejected SECRET-SERVER-TOKEN\r\n")
				return
			}
			_, _ = fmt.Fprint(conn, "250 OK\r\n")
		}
	}()
	var output bytes.Buffer
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("SMTP listener did not return a TCP address")
	}
	s := NewService(Config{Host: "127.0.0.1", Port: addr.Port, FromAddress: "no-reply@example.test"}, slog.New(slog.NewJSONHandler(&output, nil)))
	if err := s.SendContext(t.Context(), Email{To: []string{"ada@example.test"}, Subject: "Reminder"}); err == nil {
		t.Fatal("rejection did not fail send")
	}
	<-done
	for _, secret := range []string{"ada@example.test", "SECRET-SERVER-TOKEN"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("SMTP response leaked into logs: %s", output.String())
		}
	}
}

// Once the server has accepted the message, the send succeeded: a connection that
// drops before the QUIT reply (here the server hangs up; a canceled context
// closes the socket the same way) used to report it as failed, so the caller
// retried and the recipient got the email twice.
func TestSendSucceedsOnceTheMessageIsAcceptedEvenIfQuitFails(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		reply := func(s string) { _, _ = fmt.Fprint(conn, s+"\r\n") }
		reply("220 test SMTP")
		r := bufio.NewScanner(conn)
		for r.Scan() {
			switch r.Text() {
			case "DATA":
				reply("354 go ahead")
				for r.Scan() && r.Text() != "." {
				}
				reply("250 queued")
				close(accepted)
			case "QUIT":
				return // hang up without replying
			default:
				reply("250 OK")
			}
		}
	}()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("SMTP listener did not return a TCP address")
	}
	s := NewService(Config{Host: "127.0.0.1", Port: addr.Port, FromAddress: "no-reply@example.test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := s.SendContext(t.Context(), Email{To: []string{"ada@example.test"}, Subject: "Reminder", Body: "hello"}); err != nil {
		t.Fatalf("an accepted message was reported as failed: %v", err)
	}
	select {
	case <-accepted:
	default:
		t.Fatal("the server never accepted the message")
	}
}
