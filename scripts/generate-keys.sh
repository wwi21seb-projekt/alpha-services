#!/bin/bash

# Function to generate Ed25519 keys
generate_ed25519_keys() {
  # Generate an Ed25519 private key
  openssl genpkey -algorithm Ed25519 -out out/private.ed25519.key

  # Extract the public key from the private key
  openssl pkey -in out/private.ed25519.key -pubout -out out/public.ed25519.key

  echo "Ed25519 keys generated and saved to private.ed25519.key and public.ed25519.key"
}

# Function to generate VAPID keys
generate_vapid_keys() {
  # Generate EC private key in PEM format
  openssl ecparam -genkey -name prime256v1 -noout -out out/vapid-private.pem

  # Generate EC public key in PEM format
  openssl ec -in out/vapid-private.pem -pubout -out out/vapid-public.pem

  # Convert the private key to the compact format
  PRIVATE_KEY=$(openssl ec -in out/vapid-private.pem -noout -text | grep priv: -A 3 | tail -3 | tr -d '\n[:space:]' | base64 | tr -d '=' | tr '/+' '_-')

  # Convert the public key to the compact format
  PUBLIC_KEY=$(openssl ec -in out/vapid-private.pem -pubout -outform DER | tail -33 | base64 | tr -d '=' | tr '/+' '_-')

  # Save the compact keys to files
  echo "$PRIVATE_KEY" > out/vapid-private.key
  echo "$PUBLIC_KEY" > out/vapid-public.key

  rm out/vapid-private.pem out/vapid-public.pem

  echo "Compact VAPID keys saved to vapid-private.key and vapid-public.key"
}

# Function to ask if keys should be copied to helm files directory
copy_keys_to_helm() {
  # Ask the user if they want to copy the keys to ../helm/alpha-chart/files if it exists
  if [ -d "../helm/alpha-chart/files" ]; then
    read -p "Do you want to copy the keys to ../helm/alpha-chart/files? (y/N) " -n 1 -r
    echo    # move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      cp out/*.key ../helm/alpha-chart/files/
      echo "Keys copied to ../helm/alpha-chart/files"
    else
      echo "Keys not copied"
    fi
  fi
}

# Generate the keys
generate_ed25519_keys
generate_vapid_keys

# Ask if keys should be copied to helm files directory
copy_keys_to_helm
