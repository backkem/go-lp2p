# Application Examples

Examples mirroring the [openscreen-rs](https://github.com/youtube/openscreen-rs) application examples for interoperability testing.

## app-receiver

Advertises via mDNS, accepts connections, authenticates with PSK, and responds to AgentInfo requests.

```bash
go run ./openscreen-go/application/examples/app-receiver -name "My Receiver" -psk "secret"
```

## app-sender

Discovers receivers via mDNS, connects, authenticates with PSK, and requests AgentInfo.

```bash
go run ./openscreen-go/application/examples/app-sender -psk "secret"
```
