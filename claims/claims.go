// Package claims is the JWT contract between the priompt-auth issuer and the
// core server's verifier — defined once here, imported by both, so the two
// halves cannot drift.
//
//	header: {"alg":"EdDSA","typ":"JWT","kid":"<key id>"}
//	claims: sub (who), org ("" = admin, all orgs), rw (bool, may write),
//	        exp/iat (unix seconds, exp REQUIRED — issued tokens are short-lived),
//	        iss ("priompt-auth")
//
// Signature is Ed25519 over "<header>.<claims>" (base64url, no padding). The
// public keys are served at the issuer's /jwks endpoint (RFC 7517, OKP keys).
package claims

// Claims is the payload of a priompt-auth JWT.
type Claims struct {
	Sub string `json:"sub"`
	Org string `json:"org"`
	RW  bool   `json:"rw"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
	Iss string `json:"iss"`
}
