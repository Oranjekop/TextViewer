param(
    [ValidateSet('Prepare', 'Sign')]
    [string]$Mode,
    [string]$MetadataPath,
    [string]$CertificatePath,
    [string]$CertificateThumbprint,
    [string]$SignToolPath,
    [string]$PackagePath
)
$ErrorActionPreference = 'Stop'
if ($Mode -eq 'Prepare') {
    $certificate = $null
    if ($CertificateThumbprint) {
        $certificate = Get-Item -LiteralPath "Cert:\CurrentUser\My\$CertificateThumbprint"
    } elseif (Test-Path -LiteralPath $MetadataPath) {
        $previous = Get-Content -LiteralPath $MetadataPath -Raw | ConvertFrom-Json
        $certificate = Get-Item -LiteralPath "Cert:\CurrentUser\My\$($previous.Thumbprint)" -ErrorAction SilentlyContinue
    }
    if (-not $certificate -or $certificate.NotAfter -lt (Get-Date).AddDays(14)) {
        if ($CertificateThumbprint) { throw 'The selected signing certificate is missing or expires soon.' }
        $certificate = New-SelfSignedCertificate -Type CodeSigningCert `
            -Subject 'CN=TextViewer Local Development' -KeyAlgorithm RSA -KeyLength 3072 `
            -HashAlgorithm SHA256 -KeyExportPolicy NonExportable `
            -NotAfter (Get-Date).AddYears(2) -CertStoreLocation 'Cert:\CurrentUser\My'
    }
    if (-not $certificate.HasPrivateKey) { throw 'The signing certificate has no private key.' }
    New-Item -ItemType Directory -Path (Split-Path -Parent $MetadataPath) -Force | Out-Null
    $metadata = @{ Thumbprint = $certificate.Thumbprint; Publisher = $certificate.Subject }
    [IO.File]::WriteAllText($MetadataPath, ($metadata | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    Export-Certificate -Cert $certificate -FilePath $CertificatePath -Force | Out-Null
    Write-Output "Package signer: $($certificate.Subject)"
    exit 0
}
$metadata = Get-Content -LiteralPath $MetadataPath -Raw | ConvertFrom-Json
& $SignToolPath sign /fd SHA256 /sha1 $metadata.Thumbprint /s My $PackagePath
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
