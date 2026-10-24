package cast

import (
	"crypto/x509"
	_ "embed"
)

// The Cast root CAs built into openscreen's CastTrustStore
// (cast/common/certificate/cast_root_ca_cert_der-inc.h and
// eureka_root_ca_der-inc.h). Genuine Cast receivers' device certificates
// chain to one of them.
var (
	//go:embed roots/cast_root_ca.der
	castRootCADER []byte
	//go:embed roots/eureka_root_ca.der
	eurekaRootCADER []byte
)

// GoogleCastRoots returns Google's Cast root CAs ("Cast Root CA" and
// "Eureka Root CA"), for CertVerifier.Roots when talking to genuine Cast
// devices.
func GoogleCastRoots() []*x509.Certificate {
	var out []*x509.Certificate
	for _, der := range [][]byte{castRootCADER, eurekaRootCADER} {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			panic("cast: embedded root certificate: " + err.Error())
		}
		out = append(out, c)
	}
	return out
}
