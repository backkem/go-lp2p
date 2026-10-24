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

Device names carry a random suffix per run, so the example picks its own
device even if other Open Screen agents on the network use the same name.

```
[Receiver device] https://receiver-device.example wants to be discoverable as "Upload target #87d1": allowed
[Requester device] https://requester-device.example ("Uploader") shows the device picker
[Requester device] user picked Upload target #87d1 (unverified)
[Requester device] asks for the PIN shown on Upload target #87d1 (unverified)
[Receiver device] https://receiver-device.example ("Upload target #87d1") wants to accept a connection from Uploader (unverified): allowed
[Receiver device] shows PIN 1442-0279 to pair with Uploader (unverified)
[Requester device] user enters PIN 1442-0279
Receiver: got a transport
Requester: sending 4194304 bytes, sha256 88c88dc07a2f2a23…
Receiver: got 4194304 bytes in 61ms, sha256 88c88dc07a2f2a23…: ok
Requester: receiver confirmed the payload is intact
```
