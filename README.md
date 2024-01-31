# Local Peer-to-Peer API playground

This repo contains experiments to help inform the design of the [Local Peer-to-Peer API](https://github.com/WICG/local-peer-to-peer) proposal. Do not use any of this code yet, it not final, tested nor secure.

## Examples

Check out the [examples](./examples/) or run them on Replit:

<a href="https://replit.com/@backkem/go-lp2p"><img loading="lazy" src="https://replit.com/badge/github/backkem/go-lp2p" alt="Run on Replit" style="height: 40px; width: 190px;"></a>

## Open points

- LP2P API
  - [x] Initial implementation & example.
  - [x] User agent PSK present & consume
  - [x] DataChannel API & examples
  - [x] WebTransport API & examples
  - [ ] User agent peer listing & selection
- OSP(C)
  - [x] discovery, listen & dial
  - [x] data-channel protocol extension
  - [ ] WebTransport Protocol interaction
    - [x] Over OSP connection
    - [ ] Over dedicated QUIC connection
  - [ ] implement actual PAKE algorithm
- Various
  - [ ] Abstract LP2P API from underlying transport (to allow others like Wi-Fi Direct)
  - [ ] Lots of cleanup

## Coding conventions

The `lp2p` package augments a web API and favors similarity to the web API over idiomatic Go. Note that async functions are implemented as blocking. The other packages such as `ospc` should be idiomatic Go.
