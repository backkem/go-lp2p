# webtransport-pooled

The receiver's page listens for transports with `LP2PQuicTransportListener`.
The requester's page opens an `LP2PQuicTransport` on an `LP2PRequest`, which
lets the user pick the receiver and connects to it. The transport is pooled:
it shares the OSP connection rather than opening its own QUIC connection.

The requester streams 4 MiB of random data and its SHA-256 hash over a
bidirectional stream. The receiver hashes what arrived and reports back, and
the requester checks the receiver's answer.

```sh
go run ./examples/webtransport-pooled >/dev/null
```

The payload is base64 encoded and terminated with a newline. This works
around the streams shim, which zero-pads every chunk it reads and can drop
the final chunk at end of stream.

## Output

```
[Receiver device] https://receiver-device.example wants to be discoverable as "Upload target": allowed
[Requester device] https://requester-device.example ("Uploader") shows the device picker
[Requester device] user picked Upload target (unverified)
[Requester device] asks for the PIN shown on Upload target (unverified)
[Receiver device] https://receiver-device.example ("Upload target") wants to accept a connection from Uploader (unverified): allowed
[Receiver device] shows PIN 0987-7251 to pair with Uploader (unverified)
[Requester device] user enters PIN 0987-7251
Receiver: got a transport
Requester: sending 4194304 bytes, sha256 0f41cac19e9ad771…
Receiver: got 4194304 bytes in 57ms, sha256 0f41cac19e9ad771…: ok
Requester: receiver confirmed the payload is intact
```
