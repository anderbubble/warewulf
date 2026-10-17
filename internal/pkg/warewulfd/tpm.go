package warewulfd

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/go-attestation/attest"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

const (
	// TPMSecretHeader carries a node's runtime secret, hex encoded.
	TPMSecretHeader = "X-Warewulf-TPM-Secret"
	// TPMRequiredHeader on a 401 tells the node to attest with its TPM.
	TPMRequiredHeader = "X-Warewulf-TPM"

	maxEvidenceBytes = 4 << 20
)

// measuredStages are the stages whose files GRUB measures into PCR 9.
var measuredStages = map[string]bool{
	"efiboot":   true,
	"grub":      true,
	"kernel":    true,
	"image":     true,
	"system":    true,
	"initramfs": true,
}

// generateActivation makes the credential activation challenge. Tests replace
// it to learn the secret.
var generateActivation = func(p *attest.ActivationParameters) ([]byte, *attest.EncryptedCredential, error) {
	return p.Generate()
}

var nonces = newNonceStore()

// TPMNonceResponse is the body of a successful /tpm/nonce request.
type TPMNonceResponse struct {
	Nonce []byte `json:"nonce"`
	TTL   int    `json:"ttl"`
}

// TPMQuoteResponse is the body of a /tpm/quote response that issues a
// credential. For a 409 (the TPM or its baseline changed), Error names the
// change, and the node confirms it with the decrypted secret at /tpm/confirm.
type TPMQuoteResponse struct {
	Credential *attest.EncryptedCredential `json:"credential"`
	Error      tpm.Code                    `json:"error,omitempty"`
}

// TPMConfirmResponse is the body of a successful /tpm/confirm request.
type TPMConfirmResponse struct {
	// Candidate is the ID of the change recorded for approval.
	Candidate string `json:"candidate"`
}

// TPMErrorResponse is the body of a rejected TPM request.
type TPMErrorResponse struct {
	Error tpm.Code `json:"error"`
}

// HandleTPM serves /tpm/nonce/{wwid}, /tpm/quote/{wwid} and
// /tpm/confirm/{wwid}.
//
// A node requests a nonce and posts evidence quoted over it. If the evidence
// verifies, the response is a credential only its TPM can decrypt. The node
// uses the secret inside at /runtime/, or, if its TPM or baseline changed (a
// 409), presents it at /tpm/confirm to have the change recorded for approval.
func HandleTPM(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, err := initHandleRequest(w, req)
	if err != nil {
		return // response already written
	}
	id := ctx.remoteNode.Id()
	if !ctx.remoteNode.Valid() || !ctx.remoteNode.Tpm() {
		wwlog.Denied("TPM request for %s, which does not use TPM", ctx.rinfo.hwaddr)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if req.TLS == nil {
		wwlog.Denied("TPM request from %s over plain HTTP", id)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	switch ctx.rinfo.stage {
	case "tpm-nonce":
		nonce, err := nonces.issue(id, time.Now())
		if err != nil {
			wwlog.ErrorExc(err, "TPM nonce for "+id)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, TPMNonceResponse{Nonce: nonce, TTL: int(nonceTTL / time.Second)})
	case "tpm-quote":
		handleTPMQuote(w, req, ctx)
	case "tpm-confirm":
		handleTPMConfirm(w, ctx)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func handleTPMQuote(w http.ResponseWriter, req *http.Request, ctx *requestContext) {
	id := ctx.remoteNode.Id()
	now := time.Now()

	var ev tpm.Evidence
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxEvidenceBytes)).Decode(&ev); err != nil {
		wwlog.Denied("TPM evidence from %s: %s", id, err)
		writeJSON(w, http.StatusBadRequest, TPMErrorResponse{Error: tpm.CodeBadEvidence})
		return
	}
	if !nonces.consume(id, ev.Nonce, now) {
		wwlog.Denied("TPM evidence from %s: unknown, used or expired nonce", id)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var credential *attest.EncryptedCredential
	err := tpm.DefaultStore().Update(id, func(st *tpm.State) error {
		sent := st.SentFiles()
		v, err := tpm.VerifyEvidence(&ev, ev.Nonce, sent)
		if err != nil {
			st.Record(err, now)
			return err
		}
		enroll, candidate, evalErr := st.Evaluate(&ev, v, now)
		secret, ec, err := generateActivation(&attest.ActivationParameters{EK: v.EKPub, AK: ev.AK.Attestation()})
		if err != nil {
			err = &tpm.Error{Code: tpm.CodeQuoteInvalid, Err: err}
			st.Record(err, now)
			return err
		}
		credential = ec
		// Nothing is recorded until the node proves it holds the TPM; see
		// tpm.State.Evaluate.
		if candidate != nil {
			st.AddConfirmation(secret, candidate, now)
		} else {
			st.AddPending(secret, &ev, v, sent, enroll, now)
		}
		return evalErr
	})

	status := NodeStatus{NodeName: id, Stage: ctx.rinfo.stage, Ipaddr: ctx.rinfo.ipaddr}
	code := tpm.CodeOf(err)
	switch {
	case err == nil:
		wwlog.Info("TPM attestation for %s verified; issued a credential", id)
		writeJSON(w, http.StatusOK, TPMQuoteResponse{Credential: credential})
	case code == "":
		wwlog.ErrorExc(err, "TPM attestation for "+id)
		w.WriteHeader(http.StatusInternalServerError)
	case credential != nil:
		// The TPM or its baseline changed. The node must prove it holds the
		// TPM before the change is recorded.
		wwlog.Denied("TPM attestation for %s refused: %s; awaiting confirmation from the node", id, err)
		writeJSON(w, http.StatusConflict, TPMQuoteResponse{Credential: credential, Error: code})
	default:
		wwlog.Denied("TPM attestation for %s failed: %s", id, err)
		status.Sent, status.Security = strings.ToUpper(string(code)), "TPM-FAIL"
		updateStatus(&status)
		writeJSON(w, tpmHTTPStatus(code), TPMErrorResponse{Error: code})
	}
}

// handleTPMConfirm records a TPM change as a candidate for approval when the
// node presents the secret issued for it.
func handleTPMConfirm(w http.ResponseWriter, ctx *requestContext) {
	id := ctx.remoteNode.Id()
	store := tpm.DefaultStore()
	st, err := store.Load(id)
	if err != nil {
		wwlog.ErrorExc(err, "confirming TPM change for "+id)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var candidate *tpm.Candidate
	if st.HasPendingCandidate() {
		err = store.Update(id, func(st *tpm.State) error {
			candidate, _ = st.ConfirmCandidate(ctx.rinfo.tpmSecret, time.Now())
			return nil
		})
		if err != nil {
			wwlog.ErrorExc(err, "confirming TPM change for "+id)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	if candidate == nil {
		wwlog.Denied("TPM change confirmation for %s without a matching secret", id)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	cid := candidate.ID()
	wwlog.Warn("TPM change on %s (%s) recorded as candidate %s; if expected: wwctl node tpm approve %s --candidate %s",
		id, candidate.Reason, cid, id, cid)
	updateStatus(&NodeStatus{NodeName: id, Stage: ctx.rinfo.stage, Ipaddr: ctx.rinfo.ipaddr, Sent: "CANDIDATE", Security: "TPM-FAIL"})
	writeJSON(w, http.StatusOK, TPMConfirmResponse{Candidate: cid})
}

func tpmHTTPStatus(code tpm.Code) int {
	switch code {
	case tpm.CodeBadEvidence:
		return http.StatusBadRequest
	case tpm.CodeEKChanged, tpm.CodeBaselineChanged:
		return http.StatusConflict
	default:
		return http.StatusForbidden
	}
}

// tpmAuthorized checks a TPM node's runtime secret. Without TLS it refuses
// outright; otherwise a missing or wrong secret gets a 401 with
// TPMRequiredHeader, which tells the node to attest and retry.
func tpmAuthorized(w http.ResponseWriter, req *http.Request, ctx *requestContext) bool {
	id := ctx.remoteNode.Id()
	status := NodeStatus{NodeName: id, Stage: ctx.rinfo.stage, Ipaddr: ctx.rinfo.ipaddr}
	if req.TLS == nil {
		wwlog.Denied("%s requires TPM attestation, which requires TLS", id)
		w.WriteHeader(http.StatusForbidden)
		status.Sent, status.Security = "TPM_NEEDS_TLS", "TPM-FAIL"
		updateStatus(&status)
		return false
	}
	ok, err := checkTPMSecret(id, ctx.rinfo.tpmSecret, time.Now())
	if err != nil {
		wwlog.ErrorExc(err, "checking TPM secret for "+id)
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	if !ok {
		wwlog.Verbose("%s has no valid TPM secret; requesting attestation", id)
		w.Header().Set(TPMRequiredHeader, "required")
		w.WriteHeader(http.StatusUnauthorized)
		status.Sent = "TPM_REQUIRED"
		updateStatus(&status)
		return false
	}
	ctx.security = "TPM"
	return true
}

// checkTPMSecret reports whether secret is node's current or a pending
// secret. The state file is written only when a pending secret is promoted.
func checkTPMSecret(node string, secret []byte, now time.Time) (bool, error) {
	store := tpm.DefaultStore()
	st, err := store.Load(node)
	if err != nil {
		return false, err
	}
	if st.IsCurrent(secret) {
		return true, nil
	}
	if len(st.Pending) == 0 {
		return false, nil
	}
	ok := false
	err = store.Update(node, func(st *tpm.State) error {
		ok = st.CheckSecret(secret, now)
		return nil
	})
	return ok, err
}

// startTPMBoot begins a new boot session for a TPM node when it fetches its
// GRUB configuration, and records the grub.cfg served over TFTP, which GRUB
// measures before the one it fetches from this server.
func startTPMBoot(ctx *requestContext) {
	if !ctx.remoteNode.Tpm() {
		return
	}
	tftpCfg := path.Join(ctx.conf.TFTP.TftpRoot, "warewulf/grub.cfg")
	digest, digestErr := fileDigests.sumPath(tftpCfg)
	err := tpm.DefaultStore().Update(ctx.remoteNode.Id(), func(st *tpm.State) error {
		st.StartBoot(time.Now())
		if digestErr == nil {
			st.RecordSent(tpm.SentFile{Path: tftpCfg, Stage: "grub", Digest: digest})
		}
		return nil
	})
	if err != nil {
		wwlog.Warn("starting TPM boot session for %s: %s", ctx.remoteNode.Id(), err)
	}
}

// recordSent remembers a file sent to a TPM node, so its measurement is
// recognized when the node attests.
func recordSent(ctx *requestContext, stageFile, digest string) {
	err := tpm.DefaultStore().Update(ctx.remoteNode.Id(), func(st *tpm.State) error {
		st.RecordSent(tpm.SentFile{Path: stageFile, Stage: ctx.rinfo.stage, Digest: digest})
		return nil
	})
	if err != nil {
		wwlog.Warn("recording %s sent to %s: %s", stageFile, ctx.remoteNode.Id(), err)
	}
}

// measured reports whether files sent for ctx are recorded for attestation.
func measured(ctx *requestContext) bool {
	return ctx.remoteNode.Tpm() && measuredStages[ctx.rinfo.stage]
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		wwlog.ErrorExc(err, "writing response")
	}
}
