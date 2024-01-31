# Example data-channel

This example demonstrates the LP2PDataChannel API for exchanging messages.

The example simulates both peers. It is configured to run in
a non-interactive way, simulating user input.

## Example output

```
The presenting browser (receiver) show a pin:
Pin: 1234 (presented to user)
The consuming browser (requester) asks the user to enter the pin.
Pin: 1234 (entered by user)
Requester: got connection
Requester: got dataChannel
Receiver: got connection
Receiver: Received message: Good day to you, receiver!
Requester: Received message: Good day to you, requester!
```
