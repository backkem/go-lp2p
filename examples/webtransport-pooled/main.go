// Command webtransport-pooled streams a 4 MiB payload over an
// LP2PQuicTransport pooled on the OSP connection, and checks it arrived
// intact with SHA-256.
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

// receiverPage accepts transports and verifies what is streamed to it.
func receiverPage(w *Window) error {
	receiver, err := w.NewLP2PReceiver(LP2PReceiverOptions{Nickname: "Upload target"})
	if err != nil {
		return err
	}
	// Listening for transports starts the receiver.
	listener, err := NewLP2PQuicTransportListener(receiver, LP2PQuicTransportListenerInit{})
	if err != nil {
		return err
	}

	go func() {
		transports := listener.IncomingTransports.GetReader(nil).(ReadableStreamDefaultReader[*LP2PQuicTransport])
		for {
			res, err := transports.Read()
			if err != nil || res.Done {
				return
			}
			go handleTransport(res.Val)
		}
	}()
	return nil
}

func handleTransport(t *LP2PQuicTransport) {
	log.Println("Receiver: got a transport")
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
	// Opening a transport on a request lets the user pick a peer and
	// connects to it; closing the transport closes that connection.
	transport, err := NewLP2PQuicTransport(request, LP2PQuicTransportInit{AllowPooling: true})
	if err != nil {
		return err
	}
	defer transport.Close(WebTransportCloseInfo{})

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
	consumer.Pick = ua.PickByName("Upload target")
	receiverDevice := demo.NewBrowser("Receiver device", presenter)
	requesterDevice := demo.NewBrowser("Requester device", consumer)
	defer receiverDevice.Close()
	defer requesterDevice.Close()

	if err := receiverPage(receiverDevice.Window); err != nil {
		log.Fatalf("Receiver: %v", err)
	}
	if err := requesterPage(requesterDevice.Window); err != nil {
		log.Fatalf("Requester: %v", err)
	}
}
