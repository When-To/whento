#!/bin/bash

# generate-keys.sh - Generate RSA key pair for JWT signing
# Usage: ./scripts/generate-keys.sh

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

KEYS_DIR="./keys"
PRIVATE_KEY="$KEYS_DIR/private.pem"
PUBLIC_KEY="$KEYS_DIR/public.pem"

echo -e "${GREEN}Generating JWT RSA key pair...${NC}"

# Create keys directory if it doesn't exist
if [ ! -d "$KEYS_DIR" ]; then
    mkdir -p "$KEYS_DIR"
    echo -e "${GREEN}✓ Created directory: $KEYS_DIR${NC}"
fi

# Check if keys already exist
if [ -f "$PRIVATE_KEY" ] || [ -f "$PUBLIC_KEY" ]; then
    echo -e "${YELLOW}Warning: Keys already exist${NC}"
    read -p "Do you want to overwrite them? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${YELLOW}Aborted. Keeping existing keys.${NC}"
        exit 0
    fi
    echo -e "${YELLOW}Overwriting existing keys...${NC}"
fi

# Generate private key (RSA 4096 bits)
echo -e "${GREEN}Generating private key (RSA 4096)...${NC}"
openssl genrsa -out "$PRIVATE_KEY" 4096

# Generate public key from private key
echo -e "${GREEN}Generating public key...${NC}"
openssl rsa -in "$PRIVATE_KEY" -pubout -out "$PUBLIC_KEY"

# Set proper permissions (private key should be readable only by owner)
chmod 600 "$PRIVATE_KEY"
chmod 644 "$PUBLIC_KEY"

echo ""
echo -e "${GREEN}✓ JWT keys generated successfully!${NC}"
echo ""
echo "Files created:"
echo "  - Private key: $PRIVATE_KEY (permissions: 600)"
echo "  - Public key:  $PUBLIC_KEY (permissions: 644)"
echo ""
echo -e "${YELLOW}Important: Keep the private key secure and never commit it to git!${NC}"
echo ""
echo "The following entries should be in your .gitignore:"
echo "  keys/private.pem"
echo "  keys/*.pem"
