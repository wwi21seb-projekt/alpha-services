# Function to generate Ed25519 keys
function Generate-Ed25519Keys {
    # Generate an Ed25519 private key
    openssl genpkey -algorithm Ed25519 -out "out/private.ed25519.key"

    # Extract the public key from the private key
    openssl pkey -in "out/private.ed25519.key" -pubout -out "out/public.ed25519.key"

    Write-Output "Ed25519 keys generated and saved to private.ed25519.key and public.ed25519.key"
}

# Function to generate VAPID keys
function Generate-VapidKeys {
    # Generate EC private key in PEM format
    openssl ecparam -genkey -name prime256v1 -noout -out "out/vapid-private.pem"

    # Generate EC public key in PEM format
    openssl ec -in "out/vapid-private.pem" -pubout -out "out/vapid-public.pem"

    # Convert the private key to the compact format
    $privateKeyBytes = [System.Text.Encoding]::ASCII.GetBytes((openssl ec -in "out/vapid-private.pem" -noout -text | Select-String -Pattern "priv:" -Context 0,3).Context.PostContext -join "" -replace '\s', '')
    $privateKey = [Convert]::ToBase64String($privateKeyBytes).TrimEnd("=") -replace '/+', '_-', '='

    # Convert the public key to the compact format
    $publicKeyBytes = [System.IO.File]::ReadAllBytes("out/vapid-public.pem")[27..59]
    $publicKey = [Convert]::ToBase64String($publicKeyBytes).TrimEnd("=") -replace '/+', '_-', '='

    Write-Output "Compact VAPID keys generated:"
    Write-Output "Public key: $publicKey"
    Write-Output "Private key: $privateKey"

    # Save the compact keys to files
    $privateKey | Out-File -FilePath "out/vapid-private.key" -Encoding ASCII
    $publicKey | Out-File -FilePath "out/vapid-public.key" -Encoding ASCII

    Write-Output "Compact VAPID keys saved to vapid-private.key and vapid-public.key"
}

# Function to ask if keys should be copied to helm files directory
function Copy-KeysToHelm {
    if (Test-Path "../helm/alpha-chart/files") {
        $response = Read-Host "Do you want to copy the keys to ../helm/alpha-chart/files? (y/N)"
        if ($response -match '^[Yy]$') {
            Copy-Item -Path "out/private.ed25519.key" -Destination "../helm/alpha-chart/files/private.ed25519.key"
            Copy-Item -Path "out/public.ed25519.key" -Destination "../helm/alpha-chart/files/public.ed25519.key"
            Copy-Item -Path "out/vapid-private.key" -Destination "../helm/alpha-chart/files/vapid-private.key"
            Copy-Item -Path "out/vapid-public.key" -Destination "../helm/alpha-chart/files/vapid-public.key"
            Write-Output "Keys copied to ../helm/alpha-chart/files"
        } else {
            Write-Output "Keys not copied"
        }
    }
}

# Generate the keys
Generate-Ed25519Keys
Generate-VapidKeys

# Ask if keys should be copied to helm files directory
Copy-KeysToHelm
