package tpm

import "github.com/google/go-attestation/attest"

// HTTP protocol between wwclient and warewulfd.
const (
	// SecretHeader carries a node's runtime secret, hex encoded.
	SecretHeader = "X-Warewulf-TPM-Secret"
	// RequiredHeader on a 401 tells the node to attest with its TPM.
	RequiredHeader = "X-Warewulf-TPM"
)

// NonceResponse is the body of a successful /tpm/nonce request.
type NonceResponse struct {
	Nonce []byte `json:"nonce"`
	TTL   int    `json:"ttl"`
}

// QuoteResponse is the body of a /tpm/quote response that issues a
// credential. For a 409 (the TPM or its baseline changed), Error names the
// change, and the node confirms it with the decrypted secret at /tpm/confirm.
type QuoteResponse struct {
	Credential *attest.EncryptedCredential `json:"credential"`
	Error      Code                        `json:"error,omitempty"`
}

// ConfirmResponse is the body of a successful /tpm/confirm request.
type ConfirmResponse struct {
	// Candidate is the ID of the change recorded for approval.
	Candidate string `json:"candidate"`
}

// ErrorResponse is the body of a rejected TPM request.
type ErrorResponse struct {
	Error Code `json:"error"`
}
