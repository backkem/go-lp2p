# Examples

Each example runs both peers in one process and simulates the user's part of
pairing. Run them from the repository root:

- [lp2p-data-channel](./lp2p-data-channel/): exchange messages over an
  `LP2PDataChannel`.

  ```sh
  go run ./examples/lp2p-data-channel
  ```

- [webtransport-pooled](./webtransport-pooled/): stream data over an
  `LP2PQuicTransport` that shares the OSP connection.

  ```sh
  go run ./examples/webtransport-pooled
  ```
