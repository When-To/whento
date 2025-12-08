// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/whento/pkg/license"
)

var (
	// Version is set during build
	Version = "dev"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "licensegen",
		Short: "WhenTo License Generator - Generate and sign licenses for self-hosted deployments",
		Long: `WhenTo License Generator

This tool generates cryptographically signed licenses for WhenTo self-hosted deployments.
It uses Ed25519 signatures to ensure license authenticity.

Usage:
  1. Generate a key pair: licensegen keygen
  2. Generate a license: licensegen generate --tier pro --to "Company Name"`,
		Version: Version,
	}

	rootCmd.AddCommand(keygenCmd())
	rootCmd.AddCommand(generateCmd())
	rootCmd.AddCommand(renewSupportCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func keygenCmd() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate a new Ed25519 key pair for license signing",
		Long: `Generate a new Ed25519 key pair for license signing.

This will create two files:
  - license_private.key: Keep this secret! Used to sign licenses.
  - license_public.key:  Distribute this with your self-hosted binary.

The private key should be kept secure and used only by your e-commerce system
to generate licenses. The public key should be embedded in your self-hosted
WhenTo binary to verify license signatures.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateKeyPair(outputDir)
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", ".", "Output directory for key files")

	return cmd
}

func generateCmd() *cobra.Command {
	var (
		privateKeyPath string
		tier           string
		limit          int
		issuedTo       string
		output         string
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a signed license",
		Long: `Generate a signed license for a customer.

Tiers:
  - community:  30 calendars (default, no license needed)
  - pro:        300 calendars (perpetual license, 1 year support)
  - enterprise: Unlimited calendars (perpetual license, 2 years support)

The license will be output as a JSON string that can be given to the customer.
They can activate it via the API or environment variable.

Note: Self-hosted licenses are perpetual (no expiration). Only support has a time limit.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateLicense(privateKeyPath, tier, limit, issuedTo, output)
		},
	}

	cmd.Flags().StringVarP(&privateKeyPath, "key", "k", "license_private.key", "Path to private key file")
	cmd.Flags().StringVarP(&tier, "tier", "t", "", "License tier (pro, enterprise)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 0, "Calendar limit (0 = use default for tier)")
	cmd.Flags().StringVar(&issuedTo, "to", "", "License issued to (company/person name)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")

	cmd.MarkFlagRequired("tier")
	cmd.MarkFlagRequired("to")

	return cmd
}

func renewSupportCmd() *cobra.Command {
	var (
		privateKeyPath string
		licenseFile    string
		supportYears   int
		output         string
	)

	cmd := &cobra.Command{
		Use:   "renew-support",
		Short: "Renew support for an existing license",
		Long: `Renew support for an existing license.

This generates a new license with updated support period but keeps
the same tier, calendar limit, and issued_to information.

The renewed license will have:
- New support key (SUPP-XXXX-XXXX-XXXX)
- Extended support period (1 or 2 years)
- Same tier and calendar limit
- New signature

Cost: 60€/year for support renewal (all tiers)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return renewSupport(privateKeyPath, licenseFile, supportYears, output)
		},
	}

	cmd.Flags().StringVarP(&privateKeyPath, "key", "k", "license_private.key", "Path to private key file")
	cmd.Flags().StringVarP(&licenseFile, "license", "l", "", "Path to existing license JSON file")
	cmd.Flags().IntVarP(&supportYears, "years", "y", 0, "Support years (1 or 2, 0 = default based on tier)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")

	cmd.MarkFlagRequired("license")

	return cmd
}

func generateKeyPair(outputDir string) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate Ed25519 key pair using pkg/license
	publicKey, privateKey, err := license.GenerateKeyPair()
	if err != nil {
		return err
	}

	// Encode keys as base64
	publicKeyB64, privateKeyB64 := license.EncodeKeyPair(publicKey, privateKey)

	// Write private key
	privateKeyFile := fmt.Sprintf("%s/license_private.key", outputDir)
	if err := os.WriteFile(privateKeyFile, []byte(privateKeyB64), 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Write public key
	publicKeyFile := fmt.Sprintf("%s/license_public.key", outputDir)
	if err := os.WriteFile(publicKeyFile, []byte(publicKeyB64), 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	fmt.Printf("✓ Key pair generated successfully!\n\n")
	fmt.Printf("Private key: %s (keep this SECRET!)\n", privateKeyFile)
	fmt.Printf("Public key:  %s (distribute with binary)\n\n", publicKeyFile)
	fmt.Printf("Add to your self-hosted .env file:\n")
	fmt.Printf("LICENSE_PUBLIC_KEY=%s\n", publicKeyB64)

	return nil
}

func generateLicense(privateKeyPath, tier string, limit int, issuedTo string, outputFile string) error {
	// Read and decode private key
	privateKeyB64, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	privateKey, err := license.DecodePrivateKey(string(privateKeyB64))
	if err != nil {
		return err
	}

	// Generate license using pkg/license
	cfg := license.GenerateConfig{
		Tier:          tier,
		CalendarLimit: limit,
		IssuedTo:      issuedTo,
	}

	lic, err := license.Generate(cfg, privateKey)
	if err != nil {
		return err
	}

	// Marshal to JSON
	licenseJSON, err := json.MarshalIndent(lic, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal license: %w", err)
	}

	// Output
	if outputFile != "" {
		if err := os.WriteFile(outputFile, licenseJSON, 0644); err != nil {
			return fmt.Errorf("failed to write license file: %w", err)
		}
		fmt.Printf("✓ License generated successfully!\n")
		fmt.Printf("Output: %s\n\n", outputFile)
	} else {
		fmt.Printf("✓ License generated successfully!\n\n")
		fmt.Printf("License JSON (give this to customer):\n")
		fmt.Printf("%s\n\n", string(licenseJSON))
	}

	// Show summary
	fmt.Printf("License Summary:\n")
	fmt.Printf("  Tier:            %s\n", lic.Tier)
	fmt.Printf("  Calendar Limit:  ")
	if lic.CalendarLimit == 0 {
		fmt.Printf("Unlimited\n")
	} else {
		fmt.Printf("%d\n", lic.CalendarLimit)
	}
	fmt.Printf("  Issued To:       %s\n", lic.IssuedTo)
	fmt.Printf("  Issued At:       %s\n", lic.IssuedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  License Type:    Perpetual (no expiration)\n")
	fmt.Printf("\n")
	fmt.Printf("  Support Key:     %s\n", lic.SupportKey)
	if lic.SupportExpiresAt != nil {
		fmt.Printf("  Support Until:   %s\n", lic.SupportExpiresAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("  Support Until:   None\n")
	}

	fmt.Printf("\nCustomer activation instructions:\n")
	fmt.Printf("1. Add to .env file:\n")
	fmt.Printf("   LICENSE_KEY='%s'\n", string(licenseJSON))
	fmt.Printf("\n2. Or activate via API:\n")
	fmt.Printf("   POST /api/v1/license/activate\n")
	fmt.Printf("   { \"license_key\": \"%s\" }\n", string(licenseJSON))

	fmt.Printf("\nSupport key (customer can use this for support requests):\n")
	fmt.Printf("   %s\n", lic.SupportKey)
	fmt.Printf("\nCustomers can check their support status via GET /api/v1/license/info\n")
	fmt.Printf("WhenTo support team can verify this key via Cloud admin panel.\n")

	return nil
}

func renewSupport(privateKeyPath, licenseFile string, supportYears int, outputFile string) error {
	// Read existing license
	licenseJSON, err := os.ReadFile(licenseFile)
	if err != nil {
		return fmt.Errorf("failed to read license file: %w", err)
	}

	// Parse existing license
	var existingLicense license.License
	if err := json.Unmarshal(licenseJSON, &existingLicense); err != nil {
		return fmt.Errorf("failed to parse license JSON: %w", err)
	}

	// Read and decode private key
	privateKeyB64, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	privateKey, err := license.DecodePrivateKey(string(privateKeyB64))
	if err != nil {
		return err
	}

	// Renew license using pkg/license
	renewed, err := license.Renew(&existingLicense, supportYears, privateKey)
	if err != nil {
		return err
	}

	// Marshal to JSON
	renewedLicenseJSON, err := json.MarshalIndent(renewed, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal renewed license: %w", err)
	}

	// Output
	if outputFile != "" {
		if err := os.WriteFile(outputFile, renewedLicenseJSON, 0644); err != nil {
			return fmt.Errorf("failed to write renewed license file: %w", err)
		}
		fmt.Printf("✓ Support renewed successfully!\n")
		fmt.Printf("Output: %s\n\n", outputFile)
	} else {
		fmt.Printf("✓ Support renewed successfully!\n\n")
		fmt.Printf("Renewed License JSON (give this to customer):\n")
		fmt.Printf("%s\n\n", string(renewedLicenseJSON))
	}

	// Show summary
	fmt.Printf("Renewal Summary:\n")
	fmt.Printf("  Previous Support Key: %s\n", existingLicense.SupportKey)
	if existingLicense.SupportExpiresAt != nil {
		fmt.Printf("  Previous Support End: %s\n", existingLicense.SupportExpiresAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("  Previous Support End: None\n")
	}
	fmt.Printf("\n")
	fmt.Printf("  New Support Key:      %s\n", renewed.SupportKey)
	if renewed.SupportExpiresAt != nil {
		fmt.Printf("  New Support End:      %s\n", renewed.SupportExpiresAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("\n")
	fmt.Printf("  Tier:                 %s\n", renewed.Tier)
	fmt.Printf("  Calendar Limit:       ")
	if renewed.CalendarLimit == 0 {
		fmt.Printf("Unlimited\n")
	} else {
		fmt.Printf("%d\n", renewed.CalendarLimit)
	}
	fmt.Printf("  Issued To:            %s\n", renewed.IssuedTo)
	fmt.Printf("  License Type:         Perpetual (no expiration)\n")

	fmt.Printf("\nCustomer activation instructions:\n")
	fmt.Printf("1. Replace the old license in .env file with:\n")
	fmt.Printf("   LICENSE_KEY='%s'\n", string(renewedLicenseJSON))
	fmt.Printf("\n2. Or activate via API:\n")
	fmt.Printf("   POST /api/v1/license/activate\n")
	fmt.Printf("   { \"license_key\": \"%s\" }\n", string(renewedLicenseJSON))

	fmt.Printf("\nNew support key (customer can use this for support requests):\n")
	fmt.Printf("   %s\n", renewed.SupportKey)

	fmt.Printf("\nNote: The old license will be automatically replaced when the customer activates this one.\n")
	fmt.Printf("      Support renewal cost: 60€/year\n")

	return nil
}
