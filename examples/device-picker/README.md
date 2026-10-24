# device-picker

A phone's page asks for a connection while three devices (a kitchen speaker,
a living room TV and an office display) come online one by one. The phone's
browser shows a device picker that updates as the devices are discovered. The
simulated user waits until all three are listed and picks "Living room TV".

The page itself never sees the list. `LP2PRequest.start()` returns a
connection only to the device the user picked, and only that device's page
gets `onconnection`.

```sh
go run ./examples/device-picker >/dev/null
go run ./examples/device-picker -interactive >/dev/null  # pick a device yourself
```

Other Open Screen agents on your network show up in the picker too.

## Output

Device names carry a random suffix per run, so the example picks its own
device even if other Open Screen agents on the network use the same name.

```
[Phone] https://phone.example ("Phone") shows the device picker
Picker shows: []
[Kitchen speaker #d51c] https://kitchen-speaker.example wants to be discoverable as "Kitchen speaker #d51c": allowed
[Living room TV #0718] https://living-room-tv.example wants to be discoverable as "Living room TV #0718": allowed
Picker shows: [Kitchen speaker #d51c (unverified)]
[Office display #0107] https://office-display.example wants to be discoverable as "Office display #0107": allowed
Picker shows: [Kitchen speaker #d51c (unverified), Living room TV #0718 (unverified)]
Picker shows: [Kitchen speaker #d51c (unverified), Living room TV #0718 (unverified), Office display #0107 (unverified)]
[Phone] user picked Living room TV #0718 (unverified)
[Phone] asks for the PIN shown on Living room TV #0718 (unverified)
[Living room TV #0718] https://living-room-tv.example ("Living room TV #0718") wants to accept a connection from Phone (unverified): allowed
[Living room TV #0718] shows PIN 1669-8296 to pair with Phone (unverified)
[Phone] user enters PIN 1669-8296
Living room TV #0718: got a connection
Phone: "Hello from Living room TV #0718"
```

Names are "unverified" before pairing because anyone can advertise any name.
They are verified once the devices have authenticated each other.
