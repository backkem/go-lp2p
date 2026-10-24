# Open Screen test data

Test fixtures copied unchanged from the Open Screen Library.

- Source: https://chromium.googlesource.com/openscreen at commit `2a78d154`
- License: BSD-style. The source `LICENSE` begins with `// Copyright 2018 The Chromium Authors`.

## Original paths

Relative to the root of the openscreen repository:

- `test/data/cast/common/certificate/certificates/*` -> `certificates/`
- `test/data/cast/common/certificate/signeddata/*` -> `signeddata/`
- `test/data/cast/receiver/channel/{auth_challenge.pb,auth_response.pb,device_chain.pem,device_tls.pem,device_key.pem}` -> `receiver_channel/`

No file has been modified.

## Files

- `certificates/`: Cast device and CA certificate chains used by the certificate validation tests.
- `signeddata/`: MESSAGE and SHA1/SHA256 signature blocks for certain device certificates.
- `receiver_channel/`: device auth challenge and response, device certificate chain, TLS certificate, and device private key.
