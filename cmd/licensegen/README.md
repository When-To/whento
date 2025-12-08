# WhenTo License Generator

Tool for generating cryptographically signed licenses for WhenTo self-hosted deployments.

## Overview

This tool is designed to be used by your e-commerce system to generate license keys for customers who purchase WhenTo self-hosted licenses. It uses **Ed25519 digital signatures** to ensure license authenticity and prevent tampering.

## Security Model

- **Private key**: Keep this absolutely secret! Used only by your e-commerce system to sign licenses.
- **Public key**: Embedded in the self-hosted WhenTo binary to verify license signatures offline.
- **No phone-home**: Licenses are verified locally using cryptographic signatures.

## Installation

### Build from source

```bash
make build-licensegen
```

The binary will be available at `bin/licensegen`.

### Or build directly

```bash
go build -o licensegen ./cmd/licensegen
```

## Usage

### 1. Generate a key pair (one-time setup)

```bash
licensegen keygen
```

This creates two files:

- `license_private.key` - **Keep this SECRET!** Store securely in your e-commerce backend.
- `license_public.key` - Distribute with your self-hosted binary.

Example output:

```
✓ Key pair generated successfully!

Private key: ./license_private.key (keep this SECRET!)
Public key:  ./license_public.key (distribute with binary)

Add to your self-hosted .env file:
LICENSE_PUBLIC_KEY=YourBase64EncodedPublicKeyHere==
```

### 2. Generate licenses for customers

When a customer purchases a license, generate a signed license key:

#### Standard License (100 calendars, 1 year)

```bash
licensegen generate \
  --tier standard \
  --limit 100 \
  --to "ACME Corporation" \
  --expires 365
```

#### Professional License (Unlimited, perpetual)

```bash
licensegen generate \
  --tier professional \
  --to "BigCorp Inc"
```

#### Enterprise License (Unlimited, 2 years)

```bash
licensegen generate \
  --tier enterprise \
  --to "Enterprise Customer Ltd" \
  --expires 730
```

### 3. Deliver license to customer

The tool outputs a JSON license that you send to the customer:

```json
{
  "tier": "standard",
  "calendar_limit": 100,
  "issued_to": "ACME Corporation",
  "issued_at": "2025-12-01T00:00:00Z",
  "expires_at": "2026-12-01T00:00:00Z",
  "signature": "base64EncodedSignature=="
}
```

### 4. Customer activates license

Customers can activate their license in two ways:

**Option A: Environment variable (automatic activation on startup)**

```bash
# In .env file
LICENSE_KEY='{"tier":"standard","calendar_limit":100,...}'
```

**Option B: API endpoint (manual activation)**

```bash
POST /api/v1/license/activate
Authorization: Bearer <admin-token>

{
  "license_key": "{\"tier\":\"standard\",\"calendar_limit\":100,...}"
}
```

## License Tiers

| Tier         | Calendar Limit      | Typical Price            | Expiration   |
| ------------ | ------------------- | ------------------------ | ------------ |
| Community    | 20                  | Free (no license needed) | Never        |
| Standard     | 100                 | $X/year                  | Customizable |
| Professional | Unlimited           | $Y/year                  | Customizable |
| Enterprise   | Unlimited + Support | $Z/year                  | Customizable |

## Command Reference

### keygen

Generate a new Ed25519 key pair.

```bash
licensegen keygen [flags]
```

**Flags:**

- `-o, --output <dir>` - Output directory for key files (default: ".")

### generate

Generate a signed license for a customer.

```bash
licensegen generate [flags]
```

**Required flags:**

- `-t, --tier <tier>` - License tier (standard, professional, enterprise)
- `--to <name>` - License issued to (company/person name)

**Optional flags:**

- `-k, --key <path>` - Path to private key file (default: "license_private.key")
- `-l, --limit <number>` - Calendar limit (0 = unlimited, default: tier-based)
- `-e, --expires <days>` - Expires after N days (0 = perpetual, default: 0)
- `-o, --output <file>` - Output file (default: stdout)

## Integration with E-Commerce

### Recommended workflow

1. **One-time setup**: Generate key pair and securely store private key in your backend
2. **On purchase**: Customer buys a license through your e-commerce platform
3. **Generate license**: Your backend calls `licensegen` to create a signed license
4. **Deliver to customer**: Send license JSON via email or customer portal
5. **Customer activation**: Customer adds license to their self-hosted instance

### Example integration (Node.js)

```javascript
const { execSync } = require("child_process");

function generateLicense(tier, customerName, expiryDays) {
  const cmd = `licensegen generate \
    --key /secure/path/license_private.key \
    --tier ${tier} \
    --to "${customerName}" \
    --expires ${expiryDays}`;

  const licenseJSON = execSync(cmd).toString();
  return JSON.parse(licenseJSON);
}

// On purchase
const license = generateLicense("standard", "ACME Corp", 365);
sendLicenseToCustomer(license);
```

### Example integration (Go)

```go
import (
    "encoding/json"
    "os/exec"
)

func GenerateLicense(tier, customerName string, expiryDays int) (string, error) {
    cmd := exec.Command("licensegen", "generate",
        "--key", "/secure/path/license_private.key",
        "--tier", tier,
        "--to", customerName,
        "--expires", fmt.Sprintf("%d", expiryDays))

    output, err := cmd.Output()
    if err != nil {
        return "", err
    }

    return string(output), nil
}
```

## Security Best Practices

1. **Protect the private key**:

   - Store in a secure location (encrypted vault, HSM, cloud KMS)
   - Never commit to version control
   - Restrict access to authorized personnel only
   - Rotate keys periodically

2. **Audit license generation**:

   - Log all license generation events
   - Track which licenses were issued to whom
   - Monitor for suspicious patterns

3. **Customer communication**:

   - Send licenses via secure channels (encrypted email, authenticated portal)
   - Include activation instructions
   - Provide support for license issues

4. **Revocation strategy**:
   - Maintain a database of issued licenses
   - Consider implementing a revocation list for compromised licenses
   - Plan for license transfer scenarios

## Troubleshooting

### "Invalid license signature"

- Ensure the customer is using the correct public key
- Verify the license JSON wasn't modified
- Check that the license was generated with the matching private key

### "License expired"

- Check the `expires_at` field in the license
- Generate a new license with extended expiry
- Consider offering perpetual licenses (--expires 0)

## Support

For issues with the license generator tool:

- GitHub Issues: https://github.com/When-To/whento/issues
- Documentation: https://docs.whento.be

For license purchases and commercial support:

- Website: https://whento.be/pricing
- Email: sales@whento.be
