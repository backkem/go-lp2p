# Local Peer-to-Peer API playground

This repo contains experiments for the [Local Peer-to-Peer API](https://github.com/ibelem/local-peer-to-peer) proposal. Do not use any of this code yet, it not tested nor secure.

## Open points

- LP2P API
  - [x] Initial implementation & example.
  - [x] User agent PSK present & consume
  - [ ] User agent peer selection
  - [ ] WebTransport API & examples
- OSP(C)
  - [x] discovery, listen & dial
  - [x] data-channel protocol extension
  - [ ] WebTransport Protocol interaction
  - [ ] implement actual PAKE algorithm
- Various
  - [ ] Abstract LP2P API from underlying transport (to allow others like Wi-Fi Direct)
  - [ ] Lots of cleanup

## Coding conventions

The `lp2p` package augments a web API and favors similarity to the web API over idiomatic Go. The other packages such as `ospc` should be idiomatic Go.
