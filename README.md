# Local Peer-to-Peer API playground

This repo contains experiments for the [Local Peer-to-Peer API](https://github.com/ibelem/local-peer-to-peer) proposal. Do not use any of this code yet, it not tested nor secure.

## Open points

- [x] Mock out LP2P API
- [x] ospc discovery, listen & dial
- [ ] ospc PSK Authentication messages
- [ ] Finish ospc.DataChannel
- [ ] Finish LP2P API & examples
- [ ] ospc to WebTransport Protocol upgrade
- [ ] WebTransport API & examples
- [ ] Abstract LP2P API from underlying transport (to allow others like Wi-Fi Direct)
- [ ] Lots of cleanup

## Coding conventions

The `lp2p` package augments a web API and favors similarity to the web API over idiomatic Go. The other packages such as `ospc` should be idiomatic Go.
