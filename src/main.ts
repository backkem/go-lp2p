"use strict";

// This file contains stub classes to help write examples in JavaScript.

async function exampleLP2PReceiver() {
    const receiver = new LP2PReceiver({
        nickname: "example-receiver",
        // Also accept sessions requested by this origin.
        allowedOrigins: ["https://partner.example"],
    });
    receiver.onconnection = e => {
        console.log("Connection established!");
        const conn = e.connection;
        conn.onclose = () => console.log("Connection closed.");
    };

    // Blocks until permission is received.
    await receiver.start();

    // Later: stop advertising. Delivered connections stay open.
    receiver.stop();
}

exampleLP2PReceiver();

async function exampleLP2PRequest() {
    const request = new LP2PRequest({
        nickname: "example-request",
    });

    try {
        // Blocks until the user picked a peer and it accepted the session.
        const conn = await request.start();
        console.log("Connection established!");

        // Later: close the connection, its data channels and its transports.
        conn.close();
    } catch (e) {
        // NotAllowedError: dismissed, refused or failed; script can't tell which.
    }
}

exampleLP2PRequest();

async function exampleLP2PQuicTransport() {
    // Receiver
    const receiver = new LP2PReceiver({
        nickname: "example-receiver",
    });
    receiver.onconnection = async e => {
        const listener = new LP2PQuicTransportListener(e.connection);
        for await (const transport of listener.incomingTransports) {
            // Incoming transports are already ready.
            const stream = await transport.createBidirectionalStream();
        }
    };
    await receiver.start();

    // Requester
    const request = new LP2PRequest({
        nickname: "example-request",
    });
    const conn = await request.start();
    const transport = new LP2PQuicTransport(conn);
    await transport.ready;
}

exampleLP2PQuicTransport();

async function exampleLP2PDataChannel() {
    // Peer A
    const receiver = new LP2PReceiver({
        nickname: "Peer A",
    });

    receiver.onconnection = e => {
        const conn = e.connection;
        console.log("Receiver: Got a connection!");

        conn.ondatachannel = e => {
            const channel = e.channel;

            channel.onmessage = e => {
                const message = e.data;
                console.log(`Receiver: Received message: ${message}`);
            };

            channel.send("Good day to you, requester!");
        };
    };

    receiver.start();

    // Peer B
    const request = new LP2PRequest({
        nickname: "Peer B",
    });

    const conn = await request.start();
    console.log("Requester: Got a connection!");

    const channel = conn.createDataChannel("My Channel");

    channel.onopen = e => {
        channel.onmessage = e => {
            const message = e.data;
            console.log(`Requester: Received message: ${message}`);
        };

        channel.send("Good day to you, receiver!");
    };
}

exampleLP2PDataChannel();