# pairing-failures

Four ways getting a connection can fail, and what the page sees for each:

| Scenario | Page gets | `errors.Is` |
|---|---|---|
| The user won't let the page advertise | `NotAllowedError` from `receiver.start()` | `ua.ErrPermissionDenied` |
| The user dismisses the device picker | `AbortError` from `request.start()` | `ua.ErrCanceled` |
| The user types the wrong PIN | `NotAllowedError` from `request.start()` | `ua.ErrAuthenticationFailed` |
| The receiving user refuses the connection | `AbortError` from `request.start()` | `ua.ErrCanceled` |

The receiving page never hears about connections that failed or were
refused.

```sh
go run ./examples/pairing-failures >/dev/null
```

Two limitations of the OSP layer show up here:

- ospc doesn't report a PIN mismatch to either side. The user agent treats a
  pairing that doesn't finish within its `PairingTimeout` (2s here) as failed.
- ospc doesn't tell the dialing side when the other side drops the connection
  before pairing. In the last scenario the requester's user gives up waiting
  for a PIN.

## Output

Device names carry a random suffix per run, so the example picks its own
device even if other Open Screen agents on the network use the same name.

```
[Shy device] https://shy-device.example wants to be discoverable as "Shy device": denied
1. receiver.start(), advertising denied: page gets NotAllowedError (user said no)
[Indecisive phone] https://indecisive-phone.example ("Requester") shows the device picker
[Indecisive phone] user dismissed the picker
2. request.start(), picker dismissed: page gets AbortError (user canceled)
[TV #4e61] https://tv.example wants to be discoverable as "TV #4e61": allowed
[Clumsy phone] https://clumsy-phone.example ("Requester") shows the device picker
[Clumsy phone] user picked TV #4e61 (unverified)
[Clumsy phone] asks for the PIN shown on TV #4e61 (unverified)
[Clumsy phone] user enters PIN 0000-0000
[TV #4e61] https://tv.example ("TV #4e61") wants to accept a connection from Requester (unverified): allowed
[TV #4e61] shows PIN 0716-5508 to pair with Requester (unverified)
3. request.start(), wrong PIN: page gets NotAllowedError (pairing failed)
[Projector #1ecc] https://projector.example wants to be discoverable as "Projector #1ecc": allowed
[Unwelcome phone] https://unwelcome-phone.example ("Requester") shows the device picker
[Unwelcome phone] user picked Projector #1ecc (unverified)
[Unwelcome phone] asks for the PIN shown on Projector #1ecc (unverified)
[Projector #1ecc] https://projector.example ("Projector #1ecc") wants to accept a connection from Requester (unverified): denied
4. request.start(), connection refused by the receiver: page gets AbortError (user canceled)
```
