package tpm_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/google/go-tpm/legacy/tpm2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/tpm/tpmtest"
)

var (
	grubCfg = []byte("set default=single-stage")
	kernel  = []byte("vmlinuz")
	image   = []byte("image.img.gz")
)

func sent(stage string, data []byte) tpm.SentFile {
	sum := sha256.Sum256(data)
	return tpm.SentFile{Path: stage, Stage: stage, Digest: hex.EncodeToString(sum[:])}
}

// booted returns a TPM that measured firmware and a GRUB boot of the files in
// sentFiles, plus those files.
func booted(t *testing.T) (*tpmtest.TPM, []tpm.SentFile) {
	f := tpmtest.MustNew()
	for _, pcr := range []int{0, 2, 4, 7} {
		f.Extend(pcr, 0x80000008, []byte{byte(pcr)})
	}
	f.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, grubCfg)
	f.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, kernel)
	f.Extend(tpm.PCRBootFiles, tpmtest.EvIPL, image)
	return f, []tpm.SentFile{sent("grub", grubCfg), sent(tpm.StageKernel, kernel), sent("image", image)}
}

func nonce() []byte {
	n := make([]byte, 32)
	_, _ = rand.Read(n)
	return n
}

func TestVerifyEvidence(t *testing.T) {
	f, files := booted(t)
	n := nonce()
	ev, err := f.Evidence(n, nil)
	require.NoError(t, err)

	v, err := tpm.VerifyEvidence(ev, n, files)
	require.NoError(t, err)
	assert.Equal(t, tpm.Fingerprint(f.EKPub()), v.EKFingerprint)
	assert.Equal(t, f.PCR(0), v.PCRs[0])

	t.Run("EK certificate for the EK", func(t *testing.T) {
		ev, err := f.Evidence(n, nil)
		require.NoError(t, err)
		ev.EKCert = tpmtest.EKCert(&f.EK.PublicKey)
		_, err = tpm.VerifyEvidence(ev, n, files)
		assert.NoError(t, err)
	})
}

func TestVerifyEvidenceRejects(t *testing.T) {
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tests := map[string]struct {
		mutate func(f *tpmtest.TPM, ev *tpm.Evidence, n *[]byte, files *[]tpm.SentFile)
		code   tpm.Code
	}{
		"replayed nonce": {
			mutate: func(_ *tpmtest.TPM, _ *tpm.Evidence, n *[]byte, _ *[]tpm.SentFile) { *n = nonce() },
			code:   tpm.CodeQuoteInvalid,
		},
		"altered PCR value": {
			mutate: func(_ *tpmtest.TPM, ev *tpm.Evidence, _ *[]byte, _ *[]tpm.SentFile) {
				ev.PCRs[0] = make([]byte, sha256.Size)
			},
			code: tpm.CodeQuoteInvalid,
		},
		"quote signed by a different AK": {
			mutate: func(f *tpmtest.TPM, ev *tpm.Evidence, n *[]byte, _ *[]tpm.SentFile) {
				ak := ev.AK
				require.NoError(t, f.NewAK())
				q, err := f.Evidence(*n, nil)
				require.NoError(t, err)
				*ev = *q
				ev.AK = ak
			},
			code: tpm.CodeQuoteInvalid,
		},
		"missing baseline PCR": {
			mutate: func(_ *tpmtest.TPM, ev *tpm.Evidence, _ *[]byte, _ *[]tpm.SentFile) { delete(ev.PCRs, 4) },
			code:   tpm.CodeBadEvidence,
		},
		"event log does not replay": {
			mutate: func(f *tpmtest.TPM, ev *tpm.Evidence, n *[]byte, _ *[]tpm.SentFile) {
				f.ExtendSilently(tpm.PCRBootFiles, make([]byte, sha256.Size))
				q, err := f.Evidence(*n, nil)
				require.NoError(t, err)
				*ev = *q
			},
			code: tpm.CodeEventLogReplay,
		},
		"measured file not sent": {
			mutate: func(_ *tpmtest.TPM, _ *tpm.Evidence, _ *[]byte, files *[]tpm.SentFile) {
				*files = (*files)[1:]
			},
			code: tpm.CodeBootFilesUnknown,
		},
		"no kernel sent": {
			mutate: func(_ *tpmtest.TPM, _ *tpm.Evidence, _ *[]byte, files *[]tpm.SentFile) {
				for i := range *files {
					if (*files)[i].Stage == tpm.StageKernel {
						(*files)[i].Stage = "image"
					}
				}
			},
			code: tpm.CodeNoKernel,
		},
		"EK certificate for another key": {
			mutate: func(_ *tpmtest.TPM, ev *tpm.Evidence, _ *[]byte, _ *[]tpm.SentFile) {
				ev.EKCert = tpmtest.EKCert(&other.PublicKey)
			},
			code: tpm.CodeEKCertMismatch,
		},
		"garbage EK": {
			mutate: func(_ *tpmtest.TPM, ev *tpm.Evidence, _ *[]byte, _ *[]tpm.SentFile) { ev.EKPub = []byte("x") },
			code:   tpm.CodeBadEvidence,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f, files := booted(t)
			n := nonce()
			ev, err := f.Evidence(n, nil)
			require.NoError(t, err)
			tt.mutate(f, ev, &n, &files)
			_, err = tpm.VerifyEvidence(ev, n, files)
			assert.Equal(t, tt.code, tpm.CodeOf(err), "error: %v", err)
		})
	}
}

// The quote must cover every PCR the node reports. go-attestation checks only
// the PCRs in the quote's own bank, so a quote over another bank, or over
// fewer PCRs, must not vouch for the reported SHA-256 values.
func TestVerifyEvidenceUnquotedPCRs(t *testing.T) {
	tests := map[string]struct {
		alg tpm2.Algorithm
		sel []int
	}{
		"empty SHA-1 selection":      {tpm2.AlgSHA1, nil},
		"SHA-256 without PCR 9":      {tpm2.AlgSHA256, []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		"SHA-256 over baseline only": {tpm2.AlgSHA256, tpm.BaselinePCRs},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f, files := booted(t)
			n := nonce()
			ev, err := f.Evidence(n, nil)
			require.NoError(t, err)
			ev.Quote, ev.Signature, err = f.QuoteOver(n, tt.alg, tt.sel)
			require.NoError(t, err)
			_, err = tpm.VerifyEvidence(ev, n, files)
			assert.Equal(t, tpm.CodeQuoteInvalid, tpm.CodeOf(err), "error: %v", err)
		})
	}
}

// A node that did not boot through GRUB (iPXE, local disk) has nothing in
// PCR 9. Its PCR 9 replays trivially, so this must be rejected explicitly.
func TestVerifyEvidenceNoGrubBoot(t *testing.T) {
	f := tpmtest.MustNew()
	f.Extend(0, 0x80000008, []byte("firmware"))
	n := nonce()
	ev, err := f.Evidence(n, nil)
	require.NoError(t, err)
	_, err = tpm.VerifyEvidence(ev, n, []tpm.SentFile{sent(tpm.StageKernel, kernel)})
	assert.Equal(t, tpm.CodeNoBootFiles, tpm.CodeOf(err), "error: %v", err)
}

func TestDiffBaseline(t *testing.T) {
	enrolled := map[int][]byte{0: {0}, 2: {2}, 4: {4}, 7: {7}}
	current := map[int][]byte{0: {0}, 2: {2}, 4: {4}, 7: {7}, 9: {9}, 11: {11}}
	assert.Equal(t, enrolled, tpm.BaselineOf(current), "BaselineOf picks only baseline PCRs")
	assert.Empty(t, tpm.DiffBaseline(enrolled, current), "non-baseline PCRs are ignored")
	current[4] = []byte{40}
	assert.Equal(t, []int{4}, tpm.DiffBaseline(enrolled, current))
}

func TestManufacturer(t *testing.T) {
	assert.Equal(t, "Unknown", tpm.Manufacturer(nil))
	assert.Equal(t, "Unknown", tpm.Manufacturer([]byte("not a certificate")))
	f := tpmtest.MustNew()
	assert.Equal(t, "Unknown", tpm.Manufacturer(tpmtest.EKCert(&f.EK.PublicKey)), "no manufacturer attribute")
}
