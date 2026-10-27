# webtransport-pooled

The receiver's page listens for transports on each connection it receives
with `LP2PQuicTransportListener`. The requester's page connects with an
`LP2PRequest` and opens an `LP2PQuicTransport` on the connection. The
transport is pooled: it shares the OSP connection's QUIC connection, with
its own control stream, streams, datagrams and keying material exporter.
When the requester is done it closes the transport with a close code and
reason, which the receiver reads from the transport's closed promise.

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
[Receiver device] https://lp2p.example wants to be discoverable as "Upload target #c09d": allowed
[Requester device] https://lp2p.example ("Uploader") shows the device picker
[Requester device] user picked Upload target #c09d (unverified)
[Receiver device] shows PIN 012-312 to pair with Uploader (unverified)
[Requester device] asks for the PIN shown on Upload target #c09d (unverified)
[Requester device] user enters PIN 012-312
[Receiver device] https://lp2p.example ("Upload target #c09d") wants to accept a connection from Uploader (verified) on behalf of https://lp2p.example: allowed
Receiver: got a transport
Requester: sending 4194304 bytes, sha256 4fcdb1329e66b332…
Receiver: got 4194304 bytes in 74ms, sha256 4fcdb1329e66b332…: ok
Requester: receiver confirmed the payload is intact
Receiver: transport failed: WebTransportError: the session was terminated: WebTransport session closed: Application error 0x1413 (remote): Closed
```
