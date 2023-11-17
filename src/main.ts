"use strict";

// This file contains stub classes to help write examples in JavaScript.

function objectExample() {
    // Peer A
    const receiver = new LP2PReceiver({
        nickname: "Peer A",
    });

    receiver.onConnection(e => {
        const conn = e.connection;
        console.log("Receiver: Got a connection!");

        conn.onDataChannel(e => {
            const channel = e.channel;

            channel.onMessage(e => {
                const message = e.message;
                console.log(`Receiver: Received message: ${message}`);
            });

            channel.send("Good day to you, requester!");
        });
    });

    receiver.start();

    // Peer B
    const request = new LP2PRequest({
        nickname: "Peer B",
    });

    const conn = request.start();
    console.log("Requester: Got a connection!");

    const dc = conn.createDataChannel("My Channel");

    dc.onOpen(e => {
        const channel = e.channel;

        channel.onMessage(e => {
            const message = e.message;
            console.log(`Requester: Received message: ${message}`);
        });

        channel.send("Good day to you, receiver!");
    });
}

objectExample();

class LP2P {

    newReceiver(options: LP2PReceiverConfig): LP2PReceiver {
        return new LP2PReceiver(options);
    }

    newRequest(options: LP2PRequestConfig): LP2PRequest {
        return new LP2PRequest(options);
    }
}

var lp2p = new LP2P();

function globalExample() {

    // Peer A
    const receiver = lp2p.newReceiver({
        nickname: "Peer A",
    });

    receiver.onConnection(e => {
        const conn = e.connection;
        console.log("Receiver: Got a connection!");

        conn.onDataChannel(e => {
            const channel = e.channel;

            channel.onMessage(e => {
                const message = e.message;
                console.log(`Receiver: Received message: ${message}`);
            });

            channel.send("Good day to you, requester!");
        });
    });

    receiver.start();

    // Peer B
    const request = lp2p.newRequest({
        nickname: "Peer B",
    });

    const conn = request.start();
    console.log("Requester: Got a connection!");

    const dc = conn.createDataChannel("My Channel");

    dc.onOpen(e => {
        const channel = e.channel;

        channel.onMessage(e => {
            const message = e.message;
            console.log(`Requester: Received message: ${message}`);
        });

        channel.send("Good day to you, receiver!");
    });
}

objectExample();

function simplifiedExample() {
    // TODO
}

simplifiedExample();


function webTransportExample() {
    // TODO
}

webTransportExample();

function signalingExample() {
    // TODO
}

signalingExample();

type cbFn<T> = (e: T) => void;

// Receiver
interface LP2PReceiverConfig {
    nickname: string
}

interface OnConnectionEvent {
    connection: LP2PConnection
}

class LP2PReceiver {

    constructor(options: LP2PReceiverConfig) { }

    onConnection(cb: cbFn<OnConnectionEvent>) { }

    start() { }
}

// Request
interface LP2PRequestConfig {
    nickname: string
}

interface OnConnectionEvent {
    connection: LP2PConnection
}

class LP2PRequest {

    constructor(options: LP2PRequestConfig) { }

    onConnection(cb: cbFn<OnConnectionEvent>) { }

    start(): LP2PConnection { return new LP2PConnection(); }
}

// Connection
class LP2PConnection {

    createDataChannel(label: string, opts?: DataChannelInit): DataChannel { return new DataChannel(); }

    onDataChannel(cb: cbFn<OnDataChannelEvent>) { }
}

// DataChannel
interface DataChannelInit {
    protocol: string
    id: number

}

interface OnDataChannelEvent {
    channel: DataChannel
}

interface OnDataChannelOpenEvent {
    channel: DataChannel
}

interface OnMessageEvent {
    message: Payload // TODO: verify
}

type Payload = String | Blob | BinaryData;

class DataChannel {

    onMessage(cb: cbFn<OnMessageEvent>) { }

    onOpen(cb: cbFn<OnDataChannelOpenEvent>) { }

    send(message: Payload) { }
}