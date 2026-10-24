<h1 align="center">
  <img src="./.github/gopher.jpeg" alt="OpenScreen Network" height="250px">
  <br>
  Network Protocol
</h1>
<h4 align="center">OpenScreen Protocol network layer implementation</h4>
<p align="center">
  <a href="https://www.w3.org/TR/openscreen-network/">W3C Specification</a>
</p>
<br>

### Features

- QUIC transport with ALPN negotiation
- mDNS discovery and advertisement
- PSK authentication (SPAKE2)
- Agent fingerprint verification
- CBOR message encoding

### Authentication

SPAKE2 follows network.bs "Authentication with SPAKE2": identities A/B are
the client's and server's agent fingerprints, the password is the base-10 PSK
of Appendix B with dashes and leading zeros removed (`ParsePSK`, `FormatPSK`),
hashed with SHA-512, and either the PSK presenter (`psk-shown`) or the PSK
consumer (`psk-needs-presentation`) may initiate.

One deviation remains: the spec's edwards25519 suite is not implemented.
Agents must opt in to the RFC 9382 P-256 suite with
`InteropQuirks.Spake2SuiteP256` (Chromium's `osp_demo --spake2_suite=p256`);
without it `AuthenticatePSK` fails with `ErrSpake2SuiteUnsupported`.

### License

MIT License
