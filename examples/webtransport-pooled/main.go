// Command webtransport-pooled streams a 4 MiB payload over an
// LP2PQuicTransport pooled on the OSP connection, checks it arrived intact
// with SHA-256, and closes the transport with a close code the receiver
// sees.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/backkem/go-lp2p/examples/internal/demo"
	. "github.com/backkem/go-lp2p/lp2p-api" //lint:ignore ST1001 emulate the global web API
	ua "github.com/backkem/go-lp2p/lp2p-api/useragent"
	. "github.com/backkem/go-lp2p/streams-api"      //lint:ignore ST1001 emulate the global web API
	. "github.com/backkem/go-lp2p/webtransport-api" //lint:ignore ST1001 emulate the global web API
)

const payloadSize = 4 << 20

// The wire format on the stream is line based:
//
//	requester: <sha256 hex> <base64 payload>\n
//	receiver:  ok <sha256 hex>\n   or   mismatch <sha256 hex>\n
//
// Base64 and an explicit terminator work around the streams shim, which
// zero-pads every chunk it reads and may drop the last chunk at end of
// stream; both are harmless once it is fixed.

// receiverName is unique per run, so the requester picks this run's
// receiver even if other agents on the network advertise a similar name.
var receiverName = demo.Unique("Upload target")

// receiverPage accepts transports on each connection and verifies what is
// streamed to it.
func receiverPage(w *Window, done chan<- struct{}) error {
	receiver, err := w.NewLP2PReceiver(LP2PReceiverOptions{Nickname: receiverName})
	if err != nil {
		return err
	}
	receiver.OnConnection(func(e LP2PConnectionEvent) {
		// Listen for the peer's transports on this connection.
		listener, err := NewLP2PQuicTransportListener(e.Connection, LP2PQuicTransportListenerInit{})
		if err != nil {
			log.Printf("Receiver: %v", err)
			return
		}
		go func() {
			transports := listener.IncomingTransports.GetReader(nil).(ReadableStreamDefaultReader[*LP2PQuicTransport])
			for {
				res, err := transports.Read()
				if err != nil || res.Done {
					return
				}
				go handleTransport(res.Val, done)
			}
		}()
	})
	// Blocks until the user allowed the page to be discoverable.
	return receiver.Start()
}

func handleTransport(t *LP2PQuicTransport, done chan<- struct{}) {
	log.Println("Receiver: got a transport")
	go func() {
		// The closed promise: how the requester ended the transport.
		info, err := t.Closed()
		if err != nil {
			log.Printf("Receiver: transport failed: %v", err)
		} else {
			log.Printf("Receiver: transport closed by the peer with code %d: %q", info.CloseCode, info.Reason)
		}
		close(done)
	}()
	incoming := t.IncomingBidirectionalStreams.GetReader(nil).(ReadableStreamDefaultReader[WebTransportBidirectionalStream])
	for {
		res, err := incoming.Read()
		if err != nil || res.Done {
			return
		}
		go func(stream WebTransportBidirectionalStream) {
			if err := verifyUpload(stream); err != nil {
				log.Printf("Receiver: %v", err)
			}
		}(res.Val)
	}
}

func verifyUpload(stream WebTransportBidirectionalStream) error {
	start := time.Now()
	line, err := readLine(stream.Readable)
	if err != nil {
		return err
	}
	wantHash, encoded, ok := strings.Cut(line, " ")
	if !ok {
		return errors.New("malformed upload")
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	gotHash := hex.EncodeToString(sum[:])
	verdict := "ok"
	if gotHash != wantHash {
		verdict = "mismatch"
	}
	log.Printf("Receiver: got %d bytes in %s, sha256 %s…: %s",
		len(payload), time.Since(start).Round(time.Millisecond), gotHash[:16], verdict)
	return stream.Writable.GetWriter().Write([]byte(verdict + " " + gotHash + "\n"))
}

// requesterPage uploads a random payload to a peer the user picks.
func requesterPage(w *Window) error {
	request, err := w.NewLP2PRequest(LP2PRequestOptions{Nickname: "Uploader"})
	if err != nil {
		return err
	}
	// Blocks while the user picks a device and pairs with it.
	conn, err := request.Start()
	if err != nil {
		return err
	}
	defer conn.Close()

	// A transport pooled on the connection; it is ready when constructed.
	transport, err := NewLP2PQuicTransport(conn, LP2PQuicTransportInit{})
	if err != nil {
		return err
	}
	defer transport.Close(WebTransportCloseInfo{CloseCode: 0, Reason: "upload done"})

	payload := make([]byte, payloadSize)
	rand.Read(payload)
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])

	stream, err := transport.CreateBidirectionalStream()
	if err != nil {
		return err
	}
	log.Printf("Requester: sending %d bytes, sha256 %s…", len(payload), hash[:16])
	msg := hash + " " + base64.StdEncoding.EncodeToString(payload) + "\n"
	if err := stream.Writable.GetWriter().Write([]byte(msg)); err != nil {
		return err
	}

	reply, err := readLine(stream.Readable)
	if err != nil {
		return err
	}
	if reply != "ok "+hash {
		return fmt.Errorf("receiver reported %q", reply)
	}
	log.Println("Requester: receiver confirmed the payload is intact")
	return nil
}

// readLine reads from a stream up to a newline.
func readLine(rs ReadableStream[[]byte]) (string, error) {
	reader := rs.GetReader(nil).(ReadableStreamDefaultReader[[]byte])
	var buf []byte
	for {
		res, err := reader.Read()
		if err != nil {
			return "", err
		}
		if res.Done {
			return "", errors.New("stream ended before end of line")
		}
		buf = append(buf, bytes.Trim(res.Val, "\x00")...) // see the wire format note
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			return string(buf[:i]), nil
		}
	}
}

func main() {
	flag.Parse()
	demo.Watchdog(20 * time.Second)

	presenter, consumer := demo.Pair()
	consumer.Pick = ua.PickByName(receiverName)
	receiverDevice := demo.NewBrowser("Receiver device", presenter)
	requesterDevice := demo.NewUserBrowser("Requester device", consumer)
	defer receiverDevice.Close()
	defer requesterDevice.Close()

	closed := make(chan struct{})
	if err := receiverPage(receiverDevice.Window, closed); err != nil {
		log.Fatalf("Receiver: %v", err)
	}
	if err := requesterPage(requesterDevice.Window); err != nil {
		log.Fatalf("Requester: %v", err)
	}
	demo.Await(closed, "the receiver to see the transport close")
}
