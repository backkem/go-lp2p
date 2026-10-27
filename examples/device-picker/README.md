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
[Phone] https://lp2p.example ("Phone") shows the device picker
Picker shows: []
[Kitchen speaker #8bd6] https://lp2p.example wants to be discoverable as "Kitchen speaker #8bd6": allowed
Picker shows: [Kitchen speaker #8bd6 (unverified)]
[Living room TV #21b3] https://lp2p.example wants to be discoverable as "Living room TV #21b3": allowed
[Office display #2b12] https://lp2p.example wants to be discoverable as "Office display #2b12": allowed
Picker shows: [Kitchen speaker #8bd6 (unverified), Living room TV #21b3 (unverified)]
Picker shows: [Kitchen speaker #8bd6 (unverified), Living room TV #21b3 (unverified), Office display #2b12 (unverified)]
[Phone] user picked Living room TV #21b3 (unverified)
[Living room TV #21b3] shows PIN 065-447 to pair with Phone (unverified)
[Phone] asks for the PIN shown on Living room TV #21b3 (unverified)
[Phone] user enters PIN 065-447
[Living room TV #21b3] https://lp2p.example ("Living room TV #21b3") wants to accept a connection from Phone (verified) on behalf of https://lp2p.example: allowed
Living room TV #21b3: got a connection
Phone: "Hello from Living room TV #21b3"
```

Names are "unverified" before pairing because anyone can advertise any name.
They are verified once the devices have authenticated each other.
