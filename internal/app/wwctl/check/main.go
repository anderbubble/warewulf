package check

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/warewulf/warewulf/internal/pkg/check"
	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/hostlist"
	"github.com/warewulf/warewulf/internal/pkg/node"
)

func CobraRunE(cmd *cobra.Command, args []string) error {
	nodeDB, err := node.New()
	if err != nil {
		return err
	}
	nodes, err := nodeDB.FindAllNodes()
	if err != nil {
		return err
	}
	findings := check.Run(warewulfconf.Get(), nodes)

	out := cmd.OutOrStdout()
	nErrors, nWarnings := 0, 0
	for _, f := range findings {
		if f.Severity == check.Error {
			nErrors++
		} else {
			nWarnings++
		}
		_, _ = fmt.Fprintf(out, "%-5s  %s\n", f.Severity, f.Message)
		if len(f.Nodes) > 0 {
			_, _ = fmt.Fprintf(out, "       nodes: %s\n", hostlist.Compress(f.Nodes))
		}
	}
	if len(findings) == 0 {
		_, _ = fmt.Fprintln(out, "No problems found.")
		return nil
	}
	_, _ = fmt.Fprintf(out, "%d error(s), %d warning(s)\n", nErrors, nWarnings)
	if nErrors > 0 {
		return fmt.Errorf("found %d error(s)", nErrors)
	}
	return nil
}
