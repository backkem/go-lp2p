package cast

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Device certificate path validation, ported from openscreen's
// BoringSSLTrustStore::FindCertificatePath / VerifyCertificateChain
// (cast/common/certificate/boringssl_trust_store.cc).
//
// crypto/x509's Verify is not used because it diverges from what Cast
// senders accept: it refuses SHA-1 certificate signatures, which genuine
// Chromecast device and intermediate certificates use (e.g. Chromecast
// gen2, Chromecast Audio, Nexus Player), and it checks the trust anchor's
// validity period, which openscreen deliberately does not. Conversely it
// does not enforce the Cast rules openscreen adds: RSA keys of at least
// 2048 bits, RSA-only signatures, digitalSignature key usage on the device
// certificate and keyCertSign on issuers that carry key usage.

var (
	oidKeyUsage        = asn1.ObjectIdentifier{2, 5, 29, 15}
	oidBasicConstraint = asn1.ObjectIdentifier{2, 5, 29, 19}
	oidNameConstraints = asn1.ObjectIdentifier{2, 5, 29, 30}
	oidAnyPolicy       = asn1.ObjectIdentifier{2, 5, 29, 32, 0}
)

// minRSABits is kMinRsaModulusLengthBits.
const minRSABits = 2048

// verifyDeviceCertPath builds and verifies a path from target through
// intermediates to one of anchors at time now, trying candidate paths
// depth-first like openscreen. It returns the path, target first.
func verifyDeviceCertPath(target *x509.Certificate, intermediates, anchors []*x509.Certificate, now time.Time) ([]*x509.Certificate, error) {
	if err := checkTime(target, now); err != nil {
		return nil, err
	}
	if !rsaKeyLongEnough(target) {
		return nil, errors.New("device certificate key is not RSA >= 2048 bits")
	}
	if target.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return nil, errors.New("device certificate lacks digitalSignature key usage")
	}

	// openscreen sizes its path vector for every intermediate plus anchor
	// and target and derives the default max path length from that.
	maxPathLen := len(intermediates)
	var lastErr error
	var walk func(path []*x509.Certificate) []*x509.Certificate
	walk = func(path []*x509.Certificate) []*x509.Certificate {
		head := path[len(path)-1]
		for _, a := range anchors {
			if !sameName(a.RawSubject, head.RawIssuer) {
				continue
			}
			full := append(append([]*x509.Certificate{}, path...), a)
			if err := verifyCertChain(full, maxPathLen, now); err != nil {
				lastErr = err
				continue
			}
			return full
		}
		for _, c := range intermediates {
			if !sameName(c.RawSubject, head.RawIssuer) || nameInPath(c.RawSubject, path) {
				continue
			}
			if p := walk(append(append([]*x509.Certificate{}, path...), c)); p != nil {
				return p
			}
		}
		return nil
	}
	if p := walk([]*x509.Certificate{target}); p != nil {
		return p, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no path to a trusted root")
	}
	return nil, lastErr
}

// verifyCertChain is VerifyCertificateChain for path (target first, trust
// anchor last). The anchor's own validity and self-signature are not
// checked: it is trusted as configured.
func verifyCertChain(path []*x509.Certificate, maxPathLen int, now time.Time) error {
	var constraints []*nameConstraints
	for i := len(path) - 1; i > 0; i-- {
		issuer, subject := path[i], path[i-1]
		isAnchor := i == len(path)-1
		selfIssued := true
		if !isAnchor {
			if err := checkTime(issuer, now); err != nil {
				return err
			}
			if !sameName(issuer.RawSubject, issuer.RawIssuer) {
				selfIssued = false
				if maxPathLen == 0 {
					return fmt.Errorf("path length exceeded at %q", issuer.Subject.CommonName)
				}
				maxPathLen--
			}
		}
		if issuer.KeyUsage != 0 && issuer.KeyUsage&x509.KeyUsageCertSign == 0 {
			return fmt.Errorf("issuer %q lacks keyCertSign key usage", issuer.Subject.CommonName)
		}
		if !issuer.BasicConstraintsValid || !issuer.IsCA {
			return fmt.Errorf("issuer %q is not a CA", issuer.Subject.CommonName)
		}
		if issuer.MaxPathLen > 0 || issuer.MaxPathLenZero {
			if issuer.MaxPathLen > 0xff {
				return fmt.Errorf("issuer %q path length constraint too large", issuer.Subject.CommonName)
			}
			if issuer.MaxPathLen < maxPathLen {
				maxPathLen = issuer.MaxPathLen
			}
		}
		if !rsaKeyLongEnough(issuer) {
			return fmt.Errorf("issuer %q key is not RSA >= 2048 bits", issuer.Subject.CommonName)
		}
		// openscreen checks the subject against the constraints collected
		// so far when the issuer is not self-issued, and the target against
		// all of them at the end.
		if !selfIssued {
			for _, nc := range constraints {
				if err := nc.check(subject); err != nil {
					return err
				}
			}
		}
		nc, err := parseNameConstraints(issuer)
		if err != nil {
			return err
		}
		if nc != nil {
			constraints = append(constraints, nc)
		}
		for _, m := range issuer.PolicyMappings {
			if isAnyPolicy(m.IssuerDomainPolicy) || isAnyPolicy(m.SubjectDomainPolicy) {
				return fmt.Errorf("issuer %q maps anyPolicy", issuer.Subject.CommonName)
			}
		}
		for _, ext := range issuer.Extensions {
			if ext.Critical && !ext.Id.Equal(oidNameConstraints) &&
				!ext.Id.Equal(oidBasicConstraint) && !ext.Id.Equal(oidKeyUsage) {
				return fmt.Errorf("issuer %q has unhandled critical extension %v", issuer.Subject.CommonName, ext.Id)
			}
		}
		if err := checkRSASignature(subject, issuer); err != nil {
			return err
		}
	}
	for _, nc := range constraints {
		if err := nc.check(path[0]); err != nil {
			return err
		}
	}
	return nil
}

func checkTime(c *x509.Certificate, now time.Time) error {
	if now.Before(c.NotBefore) || now.After(c.NotAfter) {
		return fmt.Errorf("certificate %q not valid at %v", c.Subject.CommonName, now.UTC())
	}
	return nil
}

func rsaKeyLongEnough(c *x509.Certificate) bool {
	pub, ok := c.PublicKey.(*rsa.PublicKey)
	return ok && pub.N.BitLen() >= minRSABits
}

// checkRSASignature verifies subject's signature with issuer's key,
// accepting only RSASSA-PKCS1-v1_5 with SHA-1/256/384/512 like openscreen.
func checkRSASignature(subject, issuer *x509.Certificate) error {
	var h crypto.Hash
	switch subject.SignatureAlgorithm {
	case x509.SHA1WithRSA:
		h = crypto.SHA1
	case x509.SHA256WithRSA:
		h = crypto.SHA256
	case x509.SHA384WithRSA:
		h = crypto.SHA384
	case x509.SHA512WithRSA:
		h = crypto.SHA512
	default:
		return fmt.Errorf("certificate %q: unsupported signature algorithm %v", subject.Subject.CommonName, subject.SignatureAlgorithm)
	}
	pub, ok := issuer.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("issuer %q key is not RSA", issuer.Subject.CommonName)
	}
	w := h.New()
	w.Write(subject.RawTBSCertificate)
	if err := rsa.VerifyPKCS1v15(pub, h, w.Sum(nil), subject.Signature); err != nil {
		return fmt.Errorf("certificate %q: bad signature from %q", subject.Subject.CommonName, issuer.Subject.CommonName)
	}
	return nil
}

func isAnyPolicy(o x509.OID) bool { return o.EqualASN1OID(oidAnyPolicy) }

// sameName compares two DER-encoded distinguished names. openscreen uses
// X509_NAME_cmp, which compares canonical encodings; this compares RDNs
// attribute by attribute with case- and whitespace-insensitive string
// values, falling back to exact DER equality.
func sameName(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	ra, err1 := parseRDNs(a)
	rb, err2 := parseRDNs(b)
	if err1 != nil || err2 != nil || len(ra) != len(rb) {
		return false
	}
	return rdnPrefix(ra, rb)
}

func nameInPath(subject []byte, path []*x509.Certificate) bool {
	for _, c := range path {
		if sameName(c.RawSubject, subject) {
			return true
		}
	}
	return false
}

func parseRDNs(der []byte) (pkix.RDNSequence, error) {
	var rdns pkix.RDNSequence
	rest, err := asn1.Unmarshal(der, &rdns)
	if err == nil && len(rest) > 0 {
		err = errors.New("trailing data after name")
	}
	return rdns, err
}

// rdnPrefix reports whether prefix is a leading subsequence of name, RDN by
// RDN (RFC 5280 directoryName constraint matching).
func rdnPrefix(prefix, name pkix.RDNSequence) bool {
	if len(prefix) > len(name) {
		return false
	}
	for i, set := range prefix {
		if len(set) != len(name[i]) {
			return false
		}
		for j, atv := range set {
			other := name[i][j]
			if !atv.Type.Equal(other.Type) || canonical(atv.Value) != canonical(other.Value) {
				return false
			}
		}
	}
	return true
}

func canonical(v any) string {
	s, ok := v.(string)
	if !ok {
		return fmt.Sprint(v)
	}
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// nameConstraints holds one issuer's NameConstraints extension.
type nameConstraints struct {
	issuer                  *x509.Certificate
	permittedDN, excludedDN []pkix.RDNSequence
}

// parseNameConstraints extracts directoryName subtrees (which crypto/x509
// does not parse) and rejects forms neither it nor crypto/x509 supports.
func parseNameConstraints(c *x509.Certificate) (*nameConstraints, error) {
	var raw []byte
	for _, ext := range c.Extensions {
		if ext.Id.Equal(oidNameConstraints) {
			raw = ext.Value
		}
	}
	if raw == nil {
		return nil, nil
	}
	fail := func(err error) (*nameConstraints, error) {
		return nil, fmt.Errorf("issuer %q: name constraints: %v", c.Subject.CommonName, err)
	}
	var seq asn1.RawValue
	if rest, err := asn1.Unmarshal(raw, &seq); err != nil || len(rest) > 0 || seq.Tag != asn1.TagSequence {
		return fail(errors.New("malformed"))
	}
	nc := &nameConstraints{issuer: c}
	for b := seq.Bytes; len(b) > 0; {
		var subtrees asn1.RawValue
		var err error
		if b, err = asn1.Unmarshal(b, &subtrees); err != nil || subtrees.Class != asn1.ClassContextSpecific || subtrees.Tag > 1 {
			return fail(errors.New("malformed subtrees"))
		}
		for s := subtrees.Bytes; len(s) > 0; {
			var subtree asn1.RawValue
			if s, err = asn1.Unmarshal(s, &subtree); err != nil || subtree.Tag != asn1.TagSequence {
				return fail(errors.New("malformed subtree"))
			}
			var base asn1.RawValue
			if _, err := asn1.Unmarshal(subtree.Bytes, &base); err != nil || base.Class != asn1.ClassContextSpecific {
				return fail(errors.New("malformed general name"))
			}
			switch base.Tag {
			case 1, 2, 6, 7: // rfc822Name, dNSName, URI, iPAddress: parsed by crypto/x509
			case 4: // directoryName
				dn, err := parseRDNs(base.Bytes)
				if err != nil {
					return fail(err)
				}
				if subtrees.Tag == 0 {
					nc.permittedDN = append(nc.permittedDN, dn)
				} else {
					nc.excludedDN = append(nc.excludedDN, dn)
				}
			default:
				return fail(fmt.Errorf("unsupported general name type %d", base.Tag))
			}
		}
	}
	return nc, nil
}

// check applies the constraints to c's subject DN and subject alternative
// names (NAME_CONSTRAINTS_check).
func (nc *nameConstraints) check(c *x509.Certificate) error {
	fail := func(what string) error {
		return fmt.Errorf("certificate %q violates name constraints of %q (%s)", c.Subject.CommonName, nc.issuer.Subject.CommonName, what)
	}
	is := nc.issuer
	if subject, err := parseRDNs(c.RawSubject); err == nil && len(subject) > 0 {
		if !permitted(len(nc.permittedDN), func(i int) bool { return rdnPrefix(nc.permittedDN[i], subject) }) ||
			excluded(len(nc.excludedDN), func(i int) bool { return rdnPrefix(nc.excludedDN[i], subject) }) {
			return fail("subject")
		}
	} else if err != nil {
		return fail("unparsable subject")
	}
	for _, d := range c.DNSNames {
		if !permitted(len(is.PermittedDNSDomains), func(i int) bool { return matchDomain(d, is.PermittedDNSDomains[i]) }) ||
			excluded(len(is.ExcludedDNSDomains), func(i int) bool { return matchDomain(d, is.ExcludedDNSDomains[i]) }) {
			return fail("DNS name " + d)
		}
	}
	for _, e := range c.EmailAddresses {
		if !permitted(len(is.PermittedEmailAddresses), func(i int) bool { return matchEmail(e, is.PermittedEmailAddresses[i]) }) ||
			excluded(len(is.ExcludedEmailAddresses), func(i int) bool { return matchEmail(e, is.ExcludedEmailAddresses[i]) }) {
			return fail("email " + e)
		}
	}
	for _, ip := range c.IPAddresses {
		if !permitted(len(is.PermittedIPRanges), func(i int) bool { return is.PermittedIPRanges[i].Contains(ip) }) ||
			excluded(len(is.ExcludedIPRanges), func(i int) bool { return is.ExcludedIPRanges[i].Contains(ip) }) {
			return fail("IP " + ip.String())
		}
	}
	for _, u := range c.URIs {
		host := u.Hostname()
		if net.ParseIP(host) != nil {
			return fail("URI with IP host " + u.String())
		}
		if !permitted(len(is.PermittedURIDomains), func(i int) bool { return matchDomain(host, is.PermittedURIDomains[i]) }) ||
			excluded(len(is.ExcludedURIDomains), func(i int) bool { return matchDomain(host, is.ExcludedURIDomains[i]) }) {
			return fail("URI " + u.String())
		}
	}
	return nil
}

func permitted(n int, match func(int) bool) bool {
	if n == 0 {
		return true
	}
	for i := 0; i < n; i++ {
		if match(i) {
			return true
		}
	}
	return false
}

func excluded(n int, match func(int) bool) bool {
	for i := 0; i < n; i++ {
		if match(i) {
			return true
		}
	}
	return false
}

// matchDomain: "example.com" matches itself and subdomains, ".example.com"
// only subdomains (RFC 5280 4.2.1.10 as implemented by BoringSSL).
func matchDomain(name, constraint string) bool {
	name, constraint = strings.ToLower(name), strings.ToLower(constraint)
	if constraint == "" {
		return true
	}
	if strings.HasPrefix(constraint, ".") {
		return strings.HasSuffix(name, constraint)
	}
	return name == constraint || strings.HasSuffix(name, "."+constraint)
}

func matchEmail(addr, constraint string) bool {
	if strings.Contains(constraint, "@") {
		return strings.EqualFold(addr, constraint)
	}
	at := strings.LastIndexByte(addr, '@')
	if at < 0 {
		return false
	}
	return matchDomain(addr[at+1:], constraint)
}
