package cast

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test vectors from openscreen (testdata/openscreen, see its README):
// genuine Cast device chains and the cases of
// cast/common/certificate/cast_cert_validator_unittest.cc.

const vectorDir = "testdata/openscreen"

func readPEM(t *testing.T, path string) []*pem.Block {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(vectorDir, path))
	if err != nil {
		t.Fatal(err)
	}
	var out []*pem.Block
	for {
		var blk *pem.Block
		if blk, b = pem.Decode(b); blk == nil {
			return out
		}
		out = append(out, blk)
	}
}

func readCerts(t *testing.T, path string) []*x509.Certificate {
	t.Helper()
	var out []*x509.Certificate
	for _, blk := range readPEM(t, path) {
		if blk.Type != "CERTIFICATE" {
			continue // some files also carry the private keys
		}
		c, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out = append(out, c)
	}
	return out
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestDeviceCertPathOpenscreenVectors(t *testing.T) {
	apr2016, jan2015 := date(2016, 4, 1), date(2015, 1, 1)
	for _, tc := range []struct {
		file string
		at   time.Time
		// fileAnchor uses the last certificate of the file as the only
		// trust anchor (TRUST_STORE_FROM_TEST_FILE); otherwise
		// GoogleCastRoots (TRUST_STORE_BUILTIN).
		fileAnchor bool
		wantCN     string // "" = must fail
		signedData string
	}{
		// Genuine devices. Most use SHA-1 certificate signatures, which
		// crypto/x509's Verify (used before) rejects.
		{file: "chromecast_gen1.pem", at: apr2016, wantCN: "2ZZBG9 FA8FCA3EF91A", signedData: "2ZZBG9_FA8FCA3EF91A.pem"},
		{file: "chromecast_gen1_reissue.pem", at: apr2016, wantCN: "2ZZBG9 FA8FCA3EF91A", signedData: "2ZZBG9_FA8FCA3EF91A.pem"},
		{file: "chromecast_gen2.pem", at: apr2016, wantCN: "3ZZAK6 FA8FCA3F0D35"},
		{file: "fugu.pem", at: apr2016, wantCN: "-6394818897508095075"},
		{file: "chromecast_audio.pem", at: apr2016, wantCN: "4ZZDZJ FA8FCA7EFE3C"},
		{file: "mtk_audio_dev.pem", at: jan2015, wantCN: "MediaTek Audio Dev Test"},
		{file: "vizio.pem", at: apr2016, wantCN: "9V0000VB FA8FCA784D01"},
		{file: "audio_ref_dev_test_chain_3.pem", at: apr2016, wantCN: "Audio Reference Dev Test", signedData: "AudioReferenceDevTest.pem"},
		{file: "unchained.pem", at: apr2016},
		{file: "cast_root_ca.pem", at: apr2016}, // a root is no device cert
		{file: "chromecast_gen2.pem", at: jan2015},
		{file: "chromecast_gen2.pem", at: date(2037, 3, 1)},
		// Test PKIs.
		{file: "expired_root.pem", at: apr2016, fileAnchor: true, wantCN: "CastDevice"},
		{file: "violates_root_pathlen_constraint.pem", at: apr2016, fileAnchor: true},
		{file: "policies_ica_anypolicy_leaf_anypolicy.pem", at: apr2016, fileAnchor: true, wantCN: "Leaf"},
		{file: "policies_ica_audioonly_leaf_none.pem", at: apr2016, fileAnchor: true, wantCN: "Leaf"},
		{file: "rsa1024_device_cert.pem", at: apr2016, fileAnchor: true},
		{file: "rsa2048_device_cert.pem", at: apr2016, fileAnchor: true, wantCN: "RSA 2048 Device Cert", signedData: "rsa2048_device_cert_data.pem"},
		{file: "nc.pem", at: date(2020, 4, 1), fileAnchor: true, wantCN: "Device"},
		{file: "nc_fail.pem", at: date(2020, 4, 1), fileAnchor: true},
	} {
		t.Run(tc.file+"@"+tc.at.Format("2006-01-02"), func(t *testing.T) {
			certs := readCerts(t, "certificates/"+tc.file)
			anchors := GoogleCastRoots()
			if tc.fileAnchor {
				anchors = certs[len(certs)-1:]
				certs = certs[:len(certs)-1]
			}
			path, err := verifyDeviceCertPath(certs[0], certs[1:], anchors, tc.at)
			if tc.wantCN == "" {
				if err == nil {
					t.Fatalf("accepted, path ends at %q", path[len(path)-1].Subject.CommonName)
				}
				t.Logf("rejected: %v", err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cn := path[0].Subject.CommonName; cn != tc.wantCN {
				t.Errorf("CN %q, want %q", cn, tc.wantCN)
			}
			if tc.signedData != "" {
				checkSignedData(t, path[0], tc.signedData)
			}
		})
	}
}

// checkSignedData verifies the device key's SHA-1 and SHA-256 PKCS#1 v1.5
// signatures over MESSAGE, as openscreen's VerifySignedData tests do.
func checkSignedData(t *testing.T, leaf *x509.Certificate, file string) {
	t.Helper()
	blocks := map[string][]byte{}
	for _, blk := range readPEM(t, "signeddata/"+file) {
		blocks[blk.Type] = blk.Bytes
	}
	pub := leaf.PublicKey.(*rsa.PublicKey)
	for name, h := range map[string]crypto.Hash{"SIGNATURE SHA1": crypto.SHA1, "SIGNATURE SHA256": crypto.SHA256} {
		w := h.New()
		w.Write(blocks["MESSAGE"])
		if err := rsa.VerifyPKCS1v15(pub, h, w.Sum(nil), blocks[name]); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// openscreenAuthVector is openscreen's recorded device auth exchange
// (test/data/cast/receiver/channel): challenge, response, chain (root last),
// TLS certificate and device key.
type openscreenAuthVector struct {
	challenge *AuthChallenge
	response  []byte // serialized AuthResponse
	chain     []*x509.Certificate
	tls       *x509.Certificate
	key       *rsa.PrivateKey
}

func loadAuthVector(t *testing.T) *openscreenAuthVector {
	t.Helper()
	v := &openscreenAuthVector{}
	raw, err := os.ReadFile(filepath.Join(vectorDir, "receiver_channel/auth_challenge.pb"))
	if err != nil {
		t.Fatal(err)
	}
	var msg CastMessage
	if err := msg.Unmarshal(raw); err != nil {
		t.Fatalf("challenge CastMessage: %v", err)
	}
	if msg.Namespace != AuthNamespace || msg.PayloadType != PayloadBinary {
		t.Fatalf("unexpected challenge message %v", &msg)
	}
	var dam DeviceAuthMessage
	if err := dam.Unmarshal(msg.PayloadBinary); err != nil || dam.Challenge == nil {
		t.Fatalf("challenge DeviceAuthMessage: %v", err)
	}
	v.challenge = dam.Challenge
	if v.response, err = os.ReadFile(filepath.Join(vectorDir, "receiver_channel/auth_response.pb")); err != nil {
		t.Fatal(err)
	}
	v.chain = readCerts(t, "receiver_channel/device_chain.pem")
	v.tls = readCerts(t, "receiver_channel/device_tls.pem")[0]
	keyBlk := readPEM(t, "receiver_channel/device_key.pem")[0]
	k, err := x509.ParsePKCS1PrivateKey(keyBlk.Bytes)
	if err != nil {
		pk, err2 := x509.ParsePKCS8PrivateKey(keyBlk.Bytes)
		if err2 != nil {
			t.Fatalf("device key: %v / %v", err, err2)
		}
		k = pk.(*rsa.PrivateKey)
	}
	v.key = k
	return v
}

// TestCertVerifierOpenscreenAuthResponse verifies the AuthResponse that
// openscreen's DeviceAuthNamespaceHandler produced.
func TestCertVerifierOpenscreenAuthResponse(t *testing.T) {
	v := loadAuthVector(t)
	var resp AuthResponse
	if err := resp.unmarshal(v.response); err != nil {
		t.Fatal(err)
	}
	root := v.chain[len(v.chain)-1]
	verifier := &CertVerifier{Roots: []*x509.Certificate{root}, RequireSHA256: true,
		Now: func() time.Time { return date(2019, 12, 17) }} // device_auth_test.cc
	if err := verifier.Verify(&resp, v.challenge.SenderNonce, v.tls); err != nil {
		t.Fatal(err)
	}
	bad := append([]byte{}, v.challenge.SenderNonce...)
	bad[0] ^= 1
	if err := verifier.Verify(&resp, bad, v.tls); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("wrong nonce accepted: %v", err)
	}
}

// TestDeviceAuthResponderMatchesOpenscreen answers openscreen's recorded
// challenge with the same credentials and expects the byte-identical
// response (PKCS#1 v1.5 signatures are deterministic).
func TestDeviceAuthResponderMatchesOpenscreen(t *testing.T) {
	v := loadAuthVector(t)
	_, rs := socketPair(t, nil)
	rs.SetLocalCertificate(v.tls.Raw) // sign over openscreen's TLS cert
	r := &DeviceAuthResponder{Chain: [][]byte{v.chain[0].Raw, v.chain[1].Raw}, Key: v.key}
	resp, et, ok := r.respond(rs, v.challenge)
	if !ok {
		t.Fatalf("auth error %v", et)
	}
	got := (&DeviceAuthMessage{Response: resp}).Marshal()
	want := appendBytesField(nil, 2, v.response) // DeviceAuthMessage.response
	if !bytes.Equal(got, want) {
		t.Fatalf("response differs from openscreen's:\n got %x\nwant %x", got, want)
	}
}
