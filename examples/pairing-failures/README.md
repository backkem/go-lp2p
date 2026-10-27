# pairing-failures

Four ways getting a connection can fail, and what the page sees for each:

| Scenario | Page gets | `errors.Is` |
|---|---|---|
| The user won't let the page advertise | `NotAllowedError` from `receiver.start()` | `ua.ErrPermissionDenied` |
| The user dismisses the device picker | `NotAllowedError` from `request.start()` | `ua.ErrCanceled` |
| The user types the wrong PIN | `NotAllowedError` from `request.start()` | `ua.ErrAuthenticationFailed` |
| The receiving user refuses the connection | `NotAllowedError` from `request.start()` | `ua.ErrRefused` |

Every failure of `request.start()` is a `NotAllowedError`, as the proposal
requires, so a page can't tell a dismissed picker from a refusal; only Go
callers can, with `errors.Is`. The receiving page never hears about
connections that failed or were refused.

```sh
go run ./examples/pairing-failures >/dev/null
```

One limitation of the OSP layer shows up here: ospc doesn't report a PIN
mismatch to either side. The user agent treats a pairing that doesn't finish
within its `PairingTimeout` (2s here) as failed.

In the last scenario the receiving user is asked only after pairing, as the
proposal's "Incoming sessions" steps order it, and the refusal reaches the
requester as an `lp2p-session-response`.

## Output

Device names carry a random suffix per run, so the example picks its own
device even if other Open Screen agents on the network use the same name.

```
[Shy device] https://lp2p.example wants to be discoverable as "Shy device": denied
1. receiver.start(), advertising denied: page gets NotAllowedError (user said no)
[Indecisive phone] https://lp2p.example ("Requester") shows the device picker
[Indecisive phone] user dismissed the picker
2. request.start(), picker dismissed: page gets NotAllowedError (user canceled)
[TV #b19a] https://lp2p.example wants to be discoverable as "TV #b19a": allowed
[Clumsy phone] https://lp2p.example ("Requester") shows the device picker
[Clumsy phone] user picked TV #b19a (unverified)
[TV #b19a] shows PIN 939-869 to pair with Requester (unverified)
[Clumsy phone] asks for the PIN shown on TV #b19a (unverified)
[Clumsy phone] user enters PIN 0000-0000
3. request.start(), wrong PIN: page gets NotAllowedError (pairing failed)
[Projector #65a1] https://lp2p.example wants to be discoverable as "Projector #65a1": allowed
[Unwelcome phone] https://lp2p.example ("Requester") shows the device picker
[Unwelcome phone] user picked Projector #65a1 (unverified)
[Projector #65a1] shows PIN 230-178 to pair with Requester (unverified)
[Unwelcome phone] asks for the PIN shown on Projector #65a1 (unverified)
[Unwelcome phone] user enters PIN 230-178
[Projector #65a1] https://lp2p.example ("Projector #65a1") wants to accept a connection from Requester (verified) on behalf of https://lp2p.example: denied
4. request.start(), connection refused by the receiver: page gets NotAllowedError (refused by the peer)
```
