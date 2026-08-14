// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package email

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The sends exercised here run inside detached goroutines in production — nothing waits
// on them and nothing reports them. A connection with no timeout does not fail, it hangs,
// and one goroutine is parked per notification until the process is restarted. What these
// tests pin is that an unreachable or silent SMTP host produces an error, promptly.

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// silentServer accepts connections and then says nothing at all: no TLS handshake, no 220
// banner. This is what a wedged or blackholed SMTP host looks like from the client side,
// and it is the case a dial timeout alone does not cover.
func silentServer(t *testing.T) (host string, port int) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var (
		mu    sync.Mutex
		conns []net.Conn
	)
	done := make(chan struct{})

	// Cleanups run last-registered-first, so this one runs after the listener is closed
	// — which is what ends the accept loop.
	t.Cleanup(func() {
		<-done

		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
	})
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		defer close(done)

		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address %T", listener.Addr())
	}

	return "127.0.0.1", addr.Port
}

// closedPort returns an address nothing is listening on, so the connection is refused
// rather than left hanging.
func closedPort(t *testing.T) (host string, port int) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address %T", listener.Addr())
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	return "127.0.0.1", addr.Port
}

func TestSendFailsInsteadOfHanging(t *testing.T) {
	const (
		dialTimeout = 150 * time.Millisecond
		// The whole conversation. This is what bounds a server that accepts the
		// connection and then never speaks.
		timeout = 300 * time.Millisecond
		// Generous enough to survive a loaded CI machine, far short of the minutes an
		// unbounded connection attempt would take.
		budget = 5 * time.Second
	)

	silentHost, silentPort := silentServer(t)
	refusedHost, refusedPort := closedPort(t)

	tests := []struct {
		name string
		cfg  Config
		// wantErrContains is a fragment of the message, where the reason is worth pinning.
		wantErrContains string
	}{
		{
			name:            "no host configured",
			cfg:             Config{},
			wantErrContains: "SMTP host not configured",
		},
		{
			name: "the connection is refused",
			cfg: Config{
				Host:        refusedHost,
				Port:        refusedPort,
				FromAddress: "calendar@example.test",
				DialTimeout: dialTimeout,
				Timeout:     timeout,
			},
			wantErrContains: "failed to send email",
		},
		{
			name: "the host accepts the connection and never sends its banner",
			cfg: Config{
				Host:        silentHost,
				Port:        silentPort,
				FromAddress: "calendar@example.test",
				DialTimeout: dialTimeout,
				Timeout:     timeout,
			},
			wantErrContains: "failed to send email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(tt.cfg, discardLogger())

			done := make(chan error, 1)
			started := time.Now()

			go func() {
				done <- service.Send(Email{
					To:      []string{"participant@example.test"},
					Subject: "Everyone is free on Thursday",
					Body:    "Come along.",
				})
			}()

			select {
			case err := <-done:
				if err == nil {
					t.Fatal("the send reported success against a server that never answered")
				}
				if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("error = %q, want it to mention %q", err, tt.wantErrContains)
				}
			case <-time.After(budget):
				t.Fatalf("the send was still hanging after %v: this is the goroutine leak", time.Since(started))
			}
		})
	}
}

// TestDialDoesNotHangOnASilentHost covers the connection itself, both ways round. Port
// 465 speaks TLS from the first byte, so the handshake — not just the TCP connect — has
// to be bounded: tls.Dial with no dialer of its own waits for a ServerHello that a silent
// host will never send.
func TestDialDoesNotHangOnASilentHost(t *testing.T) {
	const dialTimeout = 150 * time.Millisecond

	host, port := silentServer(t)
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	tests := []struct {
		name string
		// port selects the connection mode; the address dialled is the silent server
		// either way.
		port    int
		wantErr bool
	}{
		{
			name:    "implicit TLS never completes its handshake",
			port:    implicitTLSPort,
			wantErr: true,
		},
		{
			name:    "a plain connection is established, and the silence comes later",
			port:    587,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(Config{
				Host:        host,
				Port:        tt.port,
				DialTimeout: dialTimeout,
			}, discardLogger())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			started := time.Now()
			conn, err := service.dial(ctx, addr)
			elapsed := time.Since(started)

			if conn != nil {
				t.Cleanup(func() { _ = conn.Close() })
			}

			switch {
			case tt.wantErr && err == nil:
				t.Fatal("the handshake reported success against a host that sent nothing")
			case !tt.wantErr && err != nil:
				t.Fatalf("dial: %v", err)
			}

			if tt.wantErr && elapsed > 2*time.Second {
				t.Errorf("gave up after %v, want about %v: the handshake is not bounded", elapsed, dialTimeout)
			}
		})
	}
}

// TestSendContextGivesUpWithTheCaller checks that a caller's own deadline is honoured
// rather than replaced by the service's, so a shutdown can drain instead of waiting out
// the full SMTP timeout.
func TestSendContextGivesUpWithTheCaller(t *testing.T) {
	host, port := silentServer(t)

	tests := []struct {
		name        string
		callerLimit time.Duration
	}{
		{name: "a caller in a hurry", callerLimit: 100 * time.Millisecond},
		{name: "a slightly more patient caller", callerLimit: 250 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(Config{
				Host:        host,
				Port:        port,
				FromAddress: "calendar@example.test",
				// Far longer than the caller is prepared to wait.
				Timeout: time.Minute,
			}, discardLogger())

			ctx, cancel := context.WithTimeout(context.Background(), tt.callerLimit)
			defer cancel()

			started := time.Now()
			err := service.SendContext(ctx, Email{
				To:      []string{"participant@example.test"},
				Subject: "Everyone is free on Thursday",
			})
			elapsed := time.Since(started)

			if err == nil {
				t.Fatal("the send reported success against a server that never answered")
			}
			if elapsed > tt.callerLimit+2*time.Second {
				t.Errorf("gave up after %v, want about %v: the caller's deadline was ignored", elapsed, tt.callerLimit)
			}
		})
	}
}

// TestNewServiceAppliesDefaultTimeouts guards the defect itself: a zero timeout is not
// "no limit configured", it is an unbounded connection attempt, and every caller of this
// package is a goroutine nobody is waiting on.
func TestNewServiceAppliesDefaultTimeouts(t *testing.T) {
	tests := []struct {
		name            string
		cfg             Config
		wantDialTimeout time.Duration
		wantTimeout     time.Duration
	}{
		{
			name:            "nothing configured",
			cfg:             Config{Host: "smtp.example.test"},
			wantDialTimeout: defaultDialTimeout,
			wantTimeout:     defaultTimeout,
		},
		{
			name:            "negative values are not taken literally",
			cfg:             Config{Host: "smtp.example.test", DialTimeout: -1, Timeout: -1},
			wantDialTimeout: defaultDialTimeout,
			wantTimeout:     defaultTimeout,
		},
		{
			name:            "explicit values win",
			cfg:             Config{Host: "smtp.example.test", DialTimeout: time.Second, Timeout: 2 * time.Second},
			wantDialTimeout: time.Second,
			wantTimeout:     2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(tt.cfg, discardLogger())

			if service.dialTimeout != tt.wantDialTimeout {
				t.Errorf("dialTimeout = %v, want %v", service.dialTimeout, tt.wantDialTimeout)
			}
			if service.timeout != tt.wantTimeout {
				t.Errorf("timeout = %v, want %v", service.timeout, tt.wantTimeout)
			}
		})
	}
}

func TestIsConfigured(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{name: "a host is set", host: "smtp.example.test", want: true},
		{name: "no host at all", host: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(Config{Host: tt.host}, discardLogger())

			if got := service.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildFromHeader(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		want    string
		wantErr bool
	}{
		{
			name: "a name and an address",
			cfg:  Config{FromName: "WhenTo", FromAddress: "calendar@example.test"},
			want: `"WhenTo" <calendar@example.test>`,
		},
		{
			// The angle brackets come from mail.Address.String, which emits the
			// canonical angle-addr form. Both spellings are valid RFC 5322 and every
			// parser takes either; this one is what the stdlib produces.
			name: "an address on its own",
			cfg:  Config{FromAddress: "calendar@example.test"},
			want: "<calendar@example.test>",
		},
		{
			// fmt.Sprintf("%s <%s>") left the comma bare, which reads as a second
			// address and produces a From header with two mailboxes in it.
			name: "a name holding a comma is quoted",
			cfg:  Config{FromName: "WhenTo, the calendar", FromAddress: "calendar@example.test"},
			want: `"WhenTo, the calendar" <calendar@example.test>`,
		},
		{
			name: "a name holding an accent is encoded",
			cfg:  Config{FromName: "Équipe WhenTo", FromAddress: "calendar@example.test"},
			want: "=?utf-8?q?=C3=89quipe_WhenTo?= <calendar@example.test>",
		},
		{
			name:    "a name carrying a newline is refused",
			cfg:     Config{FromName: "WhenTo\r\nBcc: victim@example.test", FromAddress: "calendar@example.test"},
			wantErr: true,
		},
		{
			name:    "an unparseable address is refused",
			cfg:     Config{FromAddress: "not-an-address"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(tt.cfg, discardLogger())

			got, err := service.buildFromHeader()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildFromHeader() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildFromHeader() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("buildFromHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildMessageRefusesHeaderInjection is the reason buildMessage exists as its own
// function: nothing used to observe the assembled bytes, so the header block could be
// broken open with no test to notice.
func TestBuildMessageRefusesHeaderInjection(t *testing.T) {
	tests := []struct {
		name  string
		email Email
	}{
		{
			name: "CRLF in the subject",
			email: Email{
				To:      []string{"ada@example.test"},
				Subject: "Hello\r\nBcc: victim@example.test",
			},
		},
		{
			// Go's net/textproto and several MTAs treat a bare LF as a line break, so
			// rejecting only the pair would leave the hole open.
			name: "a bare LF in the subject",
			email: Email{
				To:      []string{"ada@example.test"},
				Subject: "Hello\nBcc: victim@example.test",
			},
		},
		{
			name: "a bare CR in the subject",
			email: Email{
				To:      []string{"ada@example.test"},
				Subject: "Hello\rBcc: victim@example.test",
			},
		},
		{
			name: "CRLF in a recipient",
			email: Email{
				To:      []string{"ada@example.test\r\nBcc: victim@example.test"},
				Subject: "Hello",
			},
		},
		{
			name: "a recipient that is not an address at all",
			email: Email{
				To:      []string{"not-an-address"},
				Subject: "Hello",
			},
		},
		{
			// A display name is free text, and free text has no business in a header
			// when every caller has a validated bare address to hand.
			name: "a recipient carrying a display name",
			email: Email{
				To:      []string{`"Ada" <ada@example.test>`},
				Subject: "Hello",
			},
		},
		{
			name: "a display name is not a way to smuggle a header either",
			email: Email{
				To:      []string{`"Ada\r\nBcc: victim@example.test" <ada@example.test>`},
				Subject: "Hello",
			},
		},
		{
			name:  "no recipient",
			email: Email{Subject: "Hello"},
		},
	}

	service := NewService(Config{
		Host:        "smtp.example.test",
		FromAddress: "calendar@example.test",
	}, discardLogger())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.buildMessage(tt.email)
			if err == nil {
				t.Fatalf("buildMessage() built a message, want an error:\n%s", got)
			}
			if strings.Contains(string(got), "victim@example.test") {
				t.Errorf("the injected header reached the message:\n%s", got)
			}
		})
	}
}

func TestBuildMessageHeaders(t *testing.T) {
	service := NewService(Config{
		Host:        "smtp.example.test",
		FromName:    "WhenTo",
		FromAddress: "calendar@example.test",
	}, discardLogger())

	t.Run("a plain message", func(t *testing.T) {
		got, err := service.buildMessage(Email{
			To:      []string{"ada@example.test", "grace@example.test"},
			Subject: "Verify your WhenTo email address",
			Body:    "<p>Hello</p>",
			HTML:    true,
		})
		if err != nil {
			t.Fatalf("buildMessage() error = %v", err)
		}

		want := "From: \"WhenTo\" <calendar@example.test>\r\n" +
			"To: ada@example.test, grace@example.test\r\n" +
			"Subject: Verify your WhenTo email address\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/html; charset=UTF-8\r\n" +
			"\r\n" +
			"<p>Hello</p>\r\n"
		if string(got) != want {
			t.Errorf("buildMessage() =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("a plain-text message says so", func(t *testing.T) {
		got, err := service.buildMessage(Email{
			To:      []string{"ada@example.test"},
			Subject: "Hello",
			Body:    "Hello",
		})
		if err != nil {
			t.Fatalf("buildMessage() error = %v", err)
		}
		if !strings.Contains(string(got), "Content-Type: text/plain; charset=UTF-8") {
			t.Errorf("the message is not plain text:\n%s", got)
		}
	})

	// The French locale subjects are full of accented characters. Sent as raw UTF-8 in
	// a header, a fair share of mail clients render them as mojibake.
	t.Run("an accented subject is RFC 2047 encoded", func(t *testing.T) {
		got, err := service.buildMessage(Email{
			To:      []string{"ada@example.test"},
			Subject: "Vérifiez votre adresse email WhenTo",
		})
		if err != nil {
			t.Fatalf("buildMessage() error = %v", err)
		}

		if !strings.Contains(string(got), "Subject: =?utf-8?q?") {
			t.Errorf("the subject was not encoded:\n%s", got)
		}
		if strings.Contains(string(got), "Vérifiez") {
			t.Errorf("the subject went out as raw UTF-8:\n%s", got)
		}
	})

	// QEncoding leaves pure ASCII alone, so the English subjects are unchanged.
	t.Run("an ASCII subject is left alone", func(t *testing.T) {
		got, err := service.buildMessage(Email{
			To:      []string{"ada@example.test"},
			Subject: "Reset Your WhenTo Password",
		})
		if err != nil {
			t.Fatalf("buildMessage() error = %v", err)
		}
		if !strings.Contains(string(got), "Subject: Reset Your WhenTo Password\r\n") {
			t.Errorf("an ASCII subject was rewritten:\n%s", got)
		}
	})
}
