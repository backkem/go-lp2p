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

```
[Phone] https://phone.example ("Phone") shows the device picker
Picker shows: []
[Kitchen speaker] https://kitchen-speaker.example wants to be discoverable as "Kitchen speaker": allowed
[Living room TV] https://living-room-tv.example wants to be discoverable as "Living room TV": allowed
Picker shows: [Kitchen speaker (unverified)]
[Office display] https://office-display.example wants to be discoverable as "Office display": allowed
Picker shows: [Kitchen speaker (unverified), Living room TV (unverified)]
Picker shows: [Kitchen speaker (unverified), Living room TV (unverified), Office display (unverified)]
[Phone] user picked Living room TV (unverified)
[Living room TV] https://living-room-tv.example ("Living room TV") wants to accept a connection from Phone (unverified): allowed
[Phone] asks for the PIN shown on Living room TV (unverified)
[Living room TV] shows PIN 0720-2101 to pair with Phone (unverified)
[Phone] user enters PIN 0720-2101
Living room TV: got a connection
Phone: "Hello from Living room TV"
```

Names are "unverified" before pairing because anyone can advertise any name.
They are verified once the devices have authenticated each other.
