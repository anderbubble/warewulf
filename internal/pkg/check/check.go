// Package check finds problems in the cluster configuration that would keep
// nodes from provisioning or working as configured. Each check inspects the
// server configuration and the nodes; new cluster-wide checks are added to
// checks.
package check

import (
	"fmt"

	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
)

// Severity is how serious a Finding is.
type Severity int

const (
	// Warning is a problem that may keep a feature from working.
	Warning Severity = iota
	// Error is a problem that keeps nodes from working as configured.
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "ERROR"
	}
	return "WARN"
}

// Finding is a problem found by a check, and the nodes it affects.
type Finding struct {
	Severity Severity
	Message  string
	Nodes    []string
}

// A check inspects the configuration for one kind of problem.
type check func(conf *warewulfconf.WarewulfYaml, nodes []node.Node) []Finding

var checks = []check{tpmCheck}

// Run returns the problems all checks find.
func Run(conf *warewulfconf.WarewulfYaml, nodes []node.Node) []Finding {
	var findings []Finding
	for _, c := range checks {
		findings = append(findings, c(conf, nodes)...)
	}
	return findings
}

// tpmCheck reports server settings that TPM attestation needs and TPM
// changes awaiting approval.
func tpmCheck(conf *warewulfconf.WarewulfYaml, nodes []node.Node) []Finding {
	var tpmNodes []string
	for _, n := range nodes {
		if n.Tpm() {
			tpmNodes = append(tpmNodes, n.Id())
		}
	}
	if len(tpmNodes) == 0 {
		return nil
	}

	var findings []Finding
	for _, p := range tpm.ServerProblems(conf) {
		f := Finding{Severity: Warning, Message: p.Message, Nodes: tpmNodes}
		if p.Fatal {
			f.Severity = Error
		}
		findings = append(findings, f)
	}
	store := tpm.DefaultStore()
	for _, id := range tpmNodes {
		st, err := store.Load(id)
		if err != nil {
			findings = append(findings, Finding{Severity: Error, Message: fmt.Sprintf("reading TPM state: %s", err), Nodes: []string{id}})
			continue
		}
		if c := st.Candidate; c != nil {
			findings = append(findings, Finding{
				Severity: Warning,
				Message: fmt.Sprintf("TPM change awaiting approval (%s, candidate %s); its attestation is refused until approved. Check it with: wwctl node tpm verify %s",
					c.Reason, c.ID(), id),
				Nodes: []string{id},
			})
		}
	}
	return findings
}
