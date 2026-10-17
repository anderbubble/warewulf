package tpm

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/go-attestation/attest"
)

var (
	oidExtensionSubjectAltName = asn1.ObjectIdentifier{2, 5, 29, 17}
	oidTpmManufacturer         = asn1.ObjectIdentifier{2, 23, 133, 2, 1}
)

// TPM Manufacturer IDs from TCG Vendor ID Registry
// https://trustedcomputinggroup.org/resource/vendor-id-registry/
var tpmManufacturers = map[string]string{
	"1022": "AMD",
	"1114": "Atmel",
	"14E4": "Broadcom",
	"1137": "Cisco",
	"1A68": "Flyslice",
	"1AE0": "Google",
	"103C": "HPE",
	"19E5": "Huawei",
	"1014": "IBM",
	"15D1": "Infineon",
	"8086": "Intel",
	"17AA": "Lenovo",
	"1414": "Microsoft",
	"100B": "National Semi",
	"1B4E": "Nationz",
	"1050": "Nuvoton", // Also Winbond
	"108E": "Qualcomm",
	"1D87": "Rockchip",
	"144D": "Samsung",
	"1BFA": "Sinosun",
	"1055": "SMSC",
	"104A": "STMicroelectronics",
	"104C": "Texas Instruments",

	// Legacy / Alternate 4-byte ASCII HEX mappings (e.g. "INTC" -> 494E5443)
	"414D4400": "AMD",
	"414D4420": "AMD",
	"49465800": "Infineon",
	"49465820": "Infineon",
	"494E5443": "Intel",
	"4D534654": "Microsoft",
	"4E544300": "Nuvoton",
	"4E544320": "Nuvoton",
	"53544D20": "STMicroelectronics",
}

// Manufacturer returns the TPM vendor named in an EK certificate's subject
// alternative name (tcg-at-tpmManufacturer), or "Unknown".
func Manufacturer(ekCert []byte) string {
	if len(ekCert) == 0 {
		return "Unknown"
	}

	cert, err := attest.ParseEKCertificate(ekCert)
	if err != nil {
		return "Unknown"
	}

	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidExtensionSubjectAltName) {
			continue
		}

		var seq asn1.RawValue
		if _, err := asn1.Unmarshal(ext.Value, &seq); err != nil {
			continue
		}

		if !seq.IsCompound || seq.Tag != asn1.TagSequence {
			continue
		}

		rest := seq.Bytes
		for len(rest) > 0 {
			var v asn1.RawValue
			var err error
			rest, err = asn1.Unmarshal(rest, &v)
			if err != nil {
				continue
			}

			// directoryName has tag 4
			if v.Tag == 4 && v.Class == asn1.ClassContextSpecific {
				var rdnSeq pkix.RDNSequence
				if _, err := asn1.Unmarshal(v.Bytes, &rdnSeq); err != nil {
					continue
				}

				for _, rdn := range rdnSeq {
					for _, atv := range rdn {
						if atv.Type.Equal(oidTpmManufacturer) {
							if str, ok := atv.Value.(string); ok {
								// Format is typically "id:1022" or "id:53544D20"
								id := strings.TrimPrefix(str, "id:")
								id = strings.ToUpper(id)

								if name, ok := tpmManufacturers[id]; ok {
									return name
								}
								// Strip 0x if present
								id = strings.TrimPrefix(id, "0X")
								if name, ok := tpmManufacturers[id]; ok {
									return name
								}
								return "Unknown (" + str + ")"
							}
						}
					}
				}
			}
		}
	}

	return "Unknown"
}

// FormatEventLog renders a measurement log, limited to pcrFilter if it is
// not empty. The events are not verified.
func FormatEventLog(log []byte, pcrFilter []int) (string, error) {
	el, err := attest.ParseEventLog(log)
	if err != nil {
		return "", fmt.Errorf("parsing event log: %v", err)
	}

	events := el.Events(attest.HashSHA256)

	var sb bytes.Buffer
	sb.WriteString("TPM Event Log (SHA256):\n")
	pcrEvents := make(map[int][]string)
	for _, event := range events {
		pcrEvents[event.Index] = append(pcrEvents[event.Index], fmt.Sprintf("Type=%s Digest=%x Data=%s\n", event.Type, event.Digest, FormatEventData(event)))
	}
	for _, idx := range slices.Sorted(maps.Keys(pcrEvents)) {
		if len(pcrFilter) > 0 {
			found := false
			for _, p := range pcrFilter {
				if p == idx {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		for _, ev := range pcrEvents[idx] {
			fmt.Fprintf(&sb, "PCR[%d] %s", idx, ev)
		}
	}
	return sb.String(), nil
}
