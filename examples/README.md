# Examples

Each example runs every device in one process, each with its own mock
browser (`lp2p-api/useragent`), and simulates the users: granting
permissions, picking from the device picker, reading a PIN off one screen and
typing it on the other. Each exits on its own in a few seconds. Run them from
the repository root:

| Example | What it shows |
|---|---|
| [lp2p-data-channel](./lp2p-data-channel/) | Chat over an `LP2PDataChannel`: several messages each way, then a clean close. |
| [device-picker](./device-picker/) | Three devices appear one by one in the live device picker. The user picks one by name and only that one gets a connection. |
| [pairing-failures](./pairing-failures/) | What a page sees when the user denies advertising, dismisses the picker, types the wrong PIN or refuses a connection. |
| [webtransport-pooled](./webtransport-pooled/) | Streams 4 MiB over an `LP2PQuicTransport` and verifies it with SHA-256. |

[presentation-messages](./presentation-messages/) is different: it runs one Go
Open Screen agent against Chromium's `osp_demo` and sends the Presentation
Protocol messages (availability, start, connection messages, reconnect,
termination) by hand, printing each one. It is not a Presentation API
implementation. It needs a built `osp_demo`, and its one deviation from the
spec, `-spake2-p256`, must be turned on explicitly. See its README.

```sh
go run ./examples/lp2p-data-channel
```

ospc prints protocol debug output on stdout, and the examples write to
stderr. To see just the example, discard stdout:

```sh
go run ./examples/device-picker >/dev/null
```

`lp2p-data-channel`, `device-picker` and `webtransport-pooled` accept
`-interactive`. With it, you are the requesting user: you pick the device in
the picker and type the PIN the other device shows.

The wiring shared by the examples lives in [internal/demo](./internal/demo/).
