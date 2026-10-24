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

### Data channels

The LP2P data channel extension (local-peer-to-peer index.bs "Data channel
extension") follows the spec and interoperates with openscreen's `lp2p_demo`
(`test/lp2p-demo-interop`). Where the spec is silent, ospc does what
openscreen does:

- The acceptor answers a `data-channel-open-request` with a
  `data-channel-open-response` on the request's bidirectional stream, before
  any `data-frame`. `OpenDataChannel` returns only after a success response;
  any other result fails it with `ErrDataChannelRejected`, and no response
  within `DataChannelOpenTimeout` (or the context) fails it too.
- A `channel-id` above 65,534 is answered with `permanent-error`.
- Without an explicit ID, the QUIC client (requester side) allocates even
  IDs and the QUIC server (receiver side) odd IDs: the lowest ID of its
  parity not used by an open channel of the connection. An explicit ID that
  is in use fails with `ErrDataChannelIDInUse`, one of the peer's parity with
  `ErrDataChannelIDParity`. IDs from the peer are accepted whatever their
  parity; channels are told apart by their stream.
- Closing a channel closes the sending side of its stream (there is no close
  message).

### License

MIT License
