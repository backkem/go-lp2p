<h1 align="center">
  <img src="./.github/gopher.jpeg" alt="OpenScreen Go" height="250px">
  <br>
  OpenScreen Go
</h1>
<h4 align="center">A pure Go implementation of the OpenScreen Protocol</h4>
<p align="center">
  <a href="https://www.w3.org/TR/openscreen-network/">Network Spec</a> |
  <a href="https://www.w3.org/TR/openscreen-application/">Application Spec</a>
</p>
<p align="center">
  <a href="../LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>
<br>

### Features

- OpenScreen Protocol Network Layer
  - QUIC transport with ALPN negotiation
  - mDNS discovery and advertisement
  - PSK authentication (SPAKE2)
  - Agent fingerprint verification
- Pure Go, no Cgo

### Roadmap

- [ ] Complete Rust interoperability testing
- [ ] WebRTC transport
- [ ] Application protocol expansion

### License

MIT License - see [LICENSE](../LICENSE) for full text
