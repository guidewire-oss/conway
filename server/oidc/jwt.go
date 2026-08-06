package oidc

// Minimal RS256 JWT verification, stdlib only — mirrors the auth package's
// "sound primitives, no third-party crypto" posture. We only need to validate
// an OIDC ID token (a signed JWS), so this deliberately supports just RS256,
// the default signing algorithm for Okta, Google, Auth0, and Entra ID.

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// jwk is one key from a JWKS document (RSA only; other key types are ignored).
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"` // modulus, base64url
	E   string `json:"e"` // exponent, base64url
}

// jwks is a JSON Web Key Set.
type jwks struct {
	Keys []jwk `json:"keys"`
}

// jwtHeader is the decoded JOSE header we care about.
type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

// parseJWKS decodes a JWKS document into a map of kid -> RSA public key.
func parseJWKS(raw []byte) (map[string]*rsa.PublicKey, error) {
	var set jwks
	if err := json.Unmarshal(raw, &set); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue // only RSA/RS256 supported
		}
		pub, err := k.rsaKey()
		if err != nil {
			return nil, err
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("jwks contains no usable RSA keys")
	}
	return out, nil
}

// rsaKey reconstructs an rsa.PublicKey from the JWK's base64url n and e.
func (k jwk) rsaKey() (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.N, "="))
	if err != nil {
		return nil, fmt.Errorf("jwk %q: bad modulus: %w", k.Kid, err)
	}
	eb, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.E, "="))
	if err != nil {
		return nil, fmt.Errorf("jwk %q: bad exponent: %w", k.Kid, err)
	}
	// Exponent is a big-endian integer; left-pad to 8 bytes for binary.BigEndian.
	e := make([]byte, 8)
	copy(e[8-len(eb):], eb)
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nb),
		E: int(binary.BigEndian.Uint64(e)),
	}, nil
}

// verifySignature checks a compact JWT's RS256 signature against keys, selecting
// the key by the header's kid (falling back to the sole key when no kid is set).
// On success it returns the raw (still-encoded) claims segment for the caller to
// decode. It validates the signature only — claim checks live in the verifier.
func verifySignature(token string, keys map[string]*rsa.PublicKey) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed JWT: want 3 segments")
	}
	var hdr jwtHeader
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	if err := json.Unmarshal(hb, &hdr); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}
	if hdr.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported token algorithm %q (only RS256)", hdr.Alg)
	}
	pub := pickKey(keys, hdr.Kid)
	if pub == nil {
		return nil, fmt.Errorf("no matching signing key for kid %q", hdr.Kid)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	signed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, signed[:], sig); err != nil {
		return nil, errors.New("signature verification failed")
	}
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	return claims, nil
}

// pickKey returns the key for kid, or the only key when kid is empty and the set
// has exactly one key (common with single-key test providers).
func pickKey(keys map[string]*rsa.PublicKey, kid string) *rsa.PublicKey {
	if k, ok := keys[kid]; ok {
		return k
	}
	if kid == "" && len(keys) == 1 {
		for _, k := range keys {
			return k
		}
	}
	return nil
}
