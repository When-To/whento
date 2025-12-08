// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/smtp"
	"strings"

	"github.com/whento/pkg/license"
)

// Sender handles email delivery
type Sender struct {
	smtpHost     string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	fromEmail    string
	fromName     string
	appURL       string
	log          *slog.Logger
}

// Config holds email sender configuration
type Config struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
	AppURL       string
}

// LicenseEmail contains data for sending license emails
type LicenseEmail struct {
	To          string
	ClientName  string
	OrderID     string
	Licenses    []*license.License
	TotalAmount int // Total in cents including VAT
	VATAmount   int // VAT amount in cents
	Country     string
}

// New creates a new email sender
func New(cfg Config, log *slog.Logger) *Sender {
	return &Sender{
		smtpHost:     cfg.SMTPHost,
		smtpPort:     cfg.SMTPPort,
		smtpUsername: cfg.SMTPUsername,
		smtpPassword: cfg.SMTPPassword,
		fromEmail:    cfg.FromEmail,
		fromName:     cfg.FromName,
		appURL:       cfg.AppURL,
		log:          log,
	}
}

// Attachment represents an email attachment
type Attachment struct {
	Content  []byte
	Filename string
}

// SendLicenses sends an email with purchased licenses
func (s *Sender) SendLicenses(ctx context.Context, data LicenseEmail) error {
	// Generate HTML email
	htmlBody, err := s.generateLicenseEmailHTML(data)
	if err != nil {
		return fmt.Errorf("failed to generate email HTML: %w", err)
	}

	// Create one JSON file per license with format "Licence_type-Support_key.json"
	var attachments []Attachment
	for _, lic := range data.Licenses {
		licenseJSON, err := json.MarshalIndent(lic, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal license: %w", err)
		}

		filename := fmt.Sprintf("Licence_%s-%s.json", lic.Tier, lic.SupportKey)
		attachments = append(attachments, Attachment{
			Content:  licenseJSON,
			Filename: filename,
		})
	}

	// Build email message with multiple attachments
	subject := "Your WhenTo License Purchase"
	message := s.buildEmailWithAttachments(data.To, subject, htmlBody, attachments)

	// Send email
	auth := smtp.PlainAuth("", s.smtpUsername, s.smtpPassword, s.smtpHost)
	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)

	if err := smtp.SendMail(addr, auth, s.fromEmail, []string{data.To}, []byte(message)); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	s.log.Info("License email sent", "to", data.To, "order_id", data.OrderID, "license_count", len(data.Licenses))
	return nil
}

// generateLicenseEmailHTML generates the HTML body for license emails
func (s *Sender) generateLicenseEmailHTML(data LicenseEmail) (string, error) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 8px 8px 0 0;
            text-align: center;
        }
        .header h1 {
            margin: 0;
            font-size: 28px;
        }
        .content {
            background: #f9fafb;
            padding: 30px;
            border-radius: 0 0 8px 8px;
        }
        .summary {
            background: white;
            padding: 20px;
            border-radius: 8px;
            margin: 20px 0;
            border-left: 4px solid #667eea;
        }
        .license-item {
            background: white;
            padding: 15px;
            margin: 10px 0;
            border-radius: 6px;
            border: 1px solid #e5e7eb;
        }
        .license-tier {
            font-weight: bold;
            color: #667eea;
            font-size: 18px;
        }
        .support-key {
            font-family: 'Courier New', monospace;
            background: #f3f4f6;
            padding: 8px 12px;
            border-radius: 4px;
            display: inline-block;
            margin-top: 8px;
        }
        .button {
            display: inline-block;
            background: #667eea;
            color: white;
            padding: 12px 24px;
            text-decoration: none;
            border-radius: 6px;
            margin: 20px 0;
        }
        .footer {
            text-align: center;
            margin-top: 30px;
            padding-top: 20px;
            border-top: 1px solid #e5e7eb;
            color: #6b7280;
            font-size: 14px;
        }
        .price-line {
            display: flex;
            justify-content: space-between;
            margin: 8px 0;
        }
        .total {
            font-weight: bold;
            font-size: 18px;
            border-top: 2px solid #e5e7eb;
            padding-top: 10px;
            margin-top: 10px;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🎉 Thank You for Your Purchase!</h1>
    </div>
    <div class="content">
        <p>Dear {{.ClientName}},</p>

        <p>Thank you for purchasing WhenTo licenses! Your payment has been processed successfully.</p>

        <div class="summary">
            <h3>Order Summary</h3>
            <p><strong>Order ID:</strong> {{.OrderID}}</p>
            <p><strong>Country:</strong> {{.Country}}</p>

            <div class="price-line">
                <span>Subtotal:</span>
                <span>{{.SubtotalFormatted}}</span>
            </div>
            {{if .HasVAT}}
            <div class="price-line">
                <span>VAT ({{.VATRate}}%):</span>
                <span>{{.VATFormatted}}</span>
            </div>
            {{end}}
            <div class="price-line total">
                <span>Total Paid:</span>
                <span>{{.TotalFormatted}}</span>
            </div>
        </div>

        <h3>Your Licenses</h3>
        <p>You have purchased {{.LicenseCount}} license(s):</p>

        {{range .Licenses}}
        <div class="license-item">
            <div class="license-tier">{{.TierName}}</div>
            <p><strong>Issued to:</strong> {{.IssuedTo}}</p>
            <p><strong>Calendar Limit:</strong> {{.CalendarLimitFormatted}}</p>
            {{if .SupportExpiresAt}}
            <p><strong>Support Until:</strong> {{.SupportExpiresFormatted}}</p>
            {{else}}
            <p><strong>Support:</strong> Perpetual</p>
            {{end}}
            <div class="support-key">Support Key: {{.SupportKey}}</div>
        </div>
        {{end}}

        <h3>📥 Download Your Licenses</h3>
        <p>Your licenses are attached to this email as individual JSON files (one file per license, format: Licence_type-Support_key.json).</p>
        <p>You can also download them anytime from your order page:</p>
        <a href="{{.DownloadURL}}" class="button">Download Licenses</a>

        <h3>📖 Installation Instructions</h3>
        <ol>
            <li>Download your licenses from the attachment or the link above</li>
            <li>Add each license to your WhenTo self-hosted instance via the admin panel</li>
            <li>Or set the LICENSE_KEY environment variable with the JSON content</li>
        </ol>

        <p>For detailed installation instructions, visit our <a href="{{.DocsURL}}">documentation</a>.</p>

        <h3>💬 Need Help?</h3>
        <p>If you have any questions or need assistance, please use your support key when contacting us at <a href="mailto:support@whento.be">support@whento.be</a>.</p>

        <p>Thank you for choosing WhenTo!</p>
        <p>The WhenTo Team</p>
    </div>

    <div class="footer">
        <p>WhenTo - Collaborative Event Calendar</p>
        <p><a href="{{.AppURL}}">{{.AppURL}}</a></p>
    </div>
</body>
</html>`

	// Prepare template data
	type LicenseData struct {
		TierName                string
		IssuedTo                string
		CalendarLimitFormatted  string
		SupportKey              string
		SupportExpiresAt        bool
		SupportExpiresFormatted string
	}

	var licensesData []LicenseData
	for _, lic := range data.Licenses {
		tierName := strings.ToUpper(string(lic.Tier[0])) + lic.Tier[1:] + " License"
		calendarLimit := "Unlimited"
		if lic.CalendarLimit > 0 {
			calendarLimit = fmt.Sprintf("%d calendars", lic.CalendarLimit)
		}

		licData := LicenseData{
			TierName:               tierName,
			IssuedTo:               lic.IssuedTo,
			CalendarLimitFormatted: calendarLimit,
			SupportKey:             lic.SupportKey,
		}

		if lic.SupportExpiresAt != nil {
			licData.SupportExpiresAt = true
			licData.SupportExpiresFormatted = lic.SupportExpiresAt.Format("January 2, 2006")
		}

		licensesData = append(licensesData, licData)
	}

	// Calculate subtotal (total - VAT)
	subtotal := data.TotalAmount - data.VATAmount

	templateData := struct {
		ClientName        string
		OrderID           string
		Country           string
		SubtotalFormatted string
		HasVAT            bool
		VATRate           string
		VATFormatted      string
		TotalFormatted    string
		LicenseCount      int
		Licenses          []LicenseData
		DownloadURL       string
		DocsURL           string
		AppURL            string
	}{
		ClientName:        data.ClientName,
		OrderID:           data.OrderID,
		Country:           data.Country,
		SubtotalFormatted: formatCents(subtotal),
		HasVAT:            data.VATAmount > 0,
		VATRate:           fmt.Sprintf("%.2f", float64(data.VATAmount)/float64(subtotal)*100.0),
		VATFormatted:      formatCents(data.VATAmount),
		TotalFormatted:    formatCents(data.TotalAmount),
		LicenseCount:      len(data.Licenses),
		Licenses:          licensesData,
		DownloadURL:       fmt.Sprintf("%s/shop/orders/%s", s.appURL, data.OrderID),
		DocsURL:           fmt.Sprintf("%s/docs/licensing", s.appURL),
		AppURL:            s.appURL,
	}

	t, err := template.New("email").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, templateData); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// buildEmailWithAttachments constructs a MIME email with HTML body and multiple JSON attachments
func (s *Sender) buildEmailWithAttachments(to, subject, htmlBody string, attachments []Attachment) string {
	boundary := "boundary-whento-email"

	var message strings.Builder

	// Headers
	message.WriteString(fmt.Sprintf("From: %s <%s>\r\n", s.fromName, s.fromEmail))
	message.WriteString(fmt.Sprintf("To: %s\r\n", to))
	message.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	message.WriteString("MIME-Version: 1.0\r\n")
	message.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
	message.WriteString("\r\n")

	// HTML body part
	message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	message.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	message.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	message.WriteString("\r\n")
	message.WriteString(htmlBody)
	message.WriteString("\r\n")

	// Add each attachment
	for _, att := range attachments {
		message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		message.WriteString(fmt.Sprintf("Content-Type: application/json; name=\"%s\"\r\n", att.Filename))
		message.WriteString("Content-Transfer-Encoding: base64\r\n")
		message.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", att.Filename))
		message.WriteString("\r\n")

		// Base64 encode attachment
		encoded := encodeBase64(att.Content)
		message.WriteString(encoded)
		message.WriteString("\r\n")
	}

	// Closing boundary
	message.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return message.String()
}

// encodeBase64 encodes bytes to base64 with line wrapping at 76 characters
func encodeBase64(data []byte) string {
	const lineLength = 76
	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(data)

	// Wrap lines at 76 characters
	var result strings.Builder
	for i := 0; i < len(encoded); i += lineLength {
		end := i + lineLength
		if end > len(encoded) {
			end = len(encoded)
		}
		result.WriteString(encoded[i:end])
		result.WriteString("\r\n")
	}
	return result.String()
}

// formatCents formats cents as currency string (e.g., 10000 -> "€100.00")
func formatCents(cents int) string {
	euros := float64(cents) / 100.0
	return fmt.Sprintf("€%.2f", euros)
}
