# lp2p-data-channel

Two pages chat over an `LP2PDataChannel`. The receiver's page advertises
itself with `LP2PReceiver`. The requester's page calls `LP2PRequest.start()`,
the user picks the receiver in the device picker, and the two devices pair
with a PIN. The requester then opens a channel and sends three messages. The
receiver answers each one, and the requester closes the channel. Closing is a
handshake, so `onclose` fires on both sides.

```sh
go run ./examples/lp2p-data-channel >/dev/null
go run ./examples/lp2p-data-channel -interactive >/dev/null  # pick and type the PIN yourself
```

## Output

```
[Receiver device] https://receiver-device.example wants to be discoverable as "Chat receiver": allowed
[Requester device] https://requester-device.example ("Chat requester") shows the device picker
[Requester device] user picked Chat receiver (unverified)
[Requester device] asks for the PIN shown on Chat receiver (unverified)
[Receiver device] https://receiver-device.example ("Chat receiver") wants to accept a connection from Chat requester (unverified): allowed
[Receiver device] shows PIN 1406-6647 to pair with Chat requester (unverified)
[Requester device] user enters PIN 1406-6647
Receiver: got a connection
Requester: got a connection
Requester: channel open
Receiver: got data channel "chat"
Receiver: got "Hello receiver!"
Receiver: got "How are you?"
Receiver: got "Gotta go."
Requester: got "Hi requester, receiver here."
Requester: got "Doing fine, thanks. You?"
Requester: got "Bye!"
Receiver: channel closed
Requester: channel closed
done
```

Lines in brackets are the browsers' UI: what the simulated users see and do.
