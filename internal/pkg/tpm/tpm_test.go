package tpm

import (
	"testing"
)

func TestTpmDataEqual(t *testing.T) {
	data1 := TpmData{
		EKCert: "cert1",
		EKPub:  "ekpub1",
		AKPub:  "akpub1",
		PCRs: map[string]string{
			"1":  "pcr1",
			"8":  "pcr8_1",
			"9":  "pcr9_1",
			"10": "pcr10_1",
		},
		Quote:             "quote1",
		Signature:         "sig1",
		Nonce:             "nonce1",
		CreateData:        "cdata1",
		CreateAttestation: "cattest1",
		CreateSignature:   "csig1",
	}
	data2 := TpmData{
		EKCert: "cert1",
		EKPub:  "ekpub1",
		AKPub:  "akpub2", // Changed AKPub
		PCRs: map[string]string{
			"1":  "pcr1",
			"8":  "pcr8_2",
			"9":  "pcr9_2",
			"10": "pcr10_2",
		},
		Quote:             "quote2",
		Signature:         "sig2",
		Nonce:             "nonce2",
		CreateData:        "cdata2",
		CreateAttestation: "cattest2",
		CreateSignature:   "csig2",
	}

	if !data1.Equal(&data2) {
		t.Error("TpmData.Equal failed: should ignore AKPub, transient fields and PCRs 8, 9, 10")
	}

	data3 := TpmData{
		EKCert: "cert1",
		EKPub:  "ekpub1",
		PCRs: map[string]string{
			"1":  "pcr1_diff",
			"11": "pcr11",
		},
	}
	if !data1.Equal(&data3) {
		t.Error("TpmData.Equal failed: should ignore PCRs outside the baseline")
	}

	data3.PCRs["0"] = "pcr0_diff"
	if data1.Equal(&data3) {
		t.Error("TpmData.Equal failed: should detect different baseline PCR 0")
	}

	data4 := TpmData{
		EKCert: "cert1",
		EKPub:  "ekpub2", // Changed EKPub
		PCRs: map[string]string{
			"1": "pcr1",
		},
	}
	if data1.Equal(&data4) {
		t.Error("TpmData.Equal failed: should detect different EKPub")
	}
}

func TestTpmDataDiff(t *testing.T) {
	data1 := TpmData{
		PCRs: map[string]string{
			"1": "pcr1",
			"2": "pcr2",
		},
	}
	data2 := TpmData{
		PCRs: map[string]string{
			"1": "pcr1",
			"2": "pcr2_diff",
			"3": "pcr3_new",
		},
	}

	diff := data1.Diff(&data2)
	expected := []string{"2", "3"}

	if len(diff) != len(expected) {
		t.Errorf("TpmData.Diff failed: expected %v, got %v", expected, diff)
	}
	for i, v := range diff {
		if v != expected[i] {
			t.Errorf("TpmData.Diff failed at index %d: expected %s, got %s", i, expected[i], v)
		}
	}
}
