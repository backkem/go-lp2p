"use strict";

// This file contains stub classes to help write examples in JavaScript.

async function exampleLP2PReceiver() {
    const receiver = new LP2PReceiver({
        nickname: "example-receiver",
    });
    receiver.onconnection = e => {
        console.log("Connection established!");
        const conn = e.connection;
    };

    // Blocks until permission is received.
    await receiver.start();
}

exampleLP2PReceiver();

async function exampleLP2PRequest() {
    const request = new LP2PRequest({
        nickname: "example-request",
    });

    // Blocks until connection is received.
    const conn = await request.start();
    console.log("Connection established!");
}

exampleLP2PRequest();

async function exampleLP2PQuicTransport() {
    // Peer A
    const receiver = new LP2PReceiver({
        nickname: "Peer A",
    });

    const listener = new LP2PQuicTransportListener(receiver);

    for await (const transport of listener.incomingTransports) {
        // Blocks until transport is ready.
        await transport.ready;
    }

    // Peer B
    const request = new LP2PRequest({
        nickname: "Peer B",
    });

    const transport = new LP2PQuicTransport(request);

    // Blocks until transport is opened.
    await transport.ready;
}

exampleLP2PQuicTransport();


async function exampleLP2PQuicTransportOverConnection() {
    // Peer A
    const receiver = new LP2PReceiver({
        nickname: "Peer A",
    });
    receiver.onconnection = async e => {
        const conn = e.connection;

        const transport = new LP2PQuicTransport(conn);

        // Blocks until transport is ready.
        await transport.ready;
    };

    // Blocks until permission is received.
    await receiver.start();

    // Peer B
    const request = new LP2PRequest({
        nickname: "Peer B",
    });

    // Blocks until connection is received.
    const conn = await request.start();

    const transport = new LP2PQuicTransport(conn);

    // Blocks until transport is ready.
    await transport.ready;
}

exampleLP2PQuicTransportOverConnection();

async function exampleIncomingTransports() {
    // Peer A
    const receiver = new LP2PReceiver({
        nickname: "Peer A",
    });

    const listener = new LP2PQuicTransportListener(receiver);

    for await (const transport of listener.incomingTransports) {
        // Blocks until transport is ready.
        await transport.ready;
    }
}

exampleIncomingTransports();

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