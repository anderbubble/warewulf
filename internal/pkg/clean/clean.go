package clean

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/node"
	"github.com/warewulf/warewulf/internal/pkg/tpm"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

/*
Cleans up the OCI cache and remains of deleted nodes
*/
func CleanOciBlobCacheDir() error {
	warewulfconf := warewulfconf.Get()
	wwlog.Verbose("removing oci cache dir: %s", warewulfconf.Paths.OciBlobCachedir())
	return os.RemoveAll(warewulfconf.Paths.OciBlobCachedir())
}

func CleanOverlays() error {
	warewulfconf := warewulfconf.Get()
	nodeDB, err := node.New()
	if err != nil {
		return err
	}
	nodes := nodeDB.ListAllNodes()

	// Clean the base directory path FIRST
	baseDir := filepath.Clean(warewulfconf.Paths.OverlayProvisiondir())

	dirList, err := os.ReadDir(baseDir)
	if err != nil {
		return err
	}
	for _, item := range dirList {
		if !item.IsDir() {
			continue
		}

		if !slices.Contains(nodes, item.Name()) {

			// Construct and validate the path (filepath.Join already calls Clean)
			cleanTarget := filepath.Join(baseDir, item.Name())

			// Compute the relative path to verify cleanTarget is within baseDir.
			// This should never fail in normal operation since we just constructed cleanTarget
			// using filepath.Join(baseDir, item.Name()). However, filepath.Rel() can fail in
			// edge cases (different volumes on Windows, symlink anomalies, filesystem corruption).
			// If we cannot determine the path relationship, we cannot safely validate whether
			// deletion would stay within bounds, so we fail-secure and return an error.
			rel, err := filepath.Rel(baseDir, cleanTarget)
			if err != nil {
				return fmt.Errorf(
					"failed to compute relative path for '%s' with overlay working directory '%s': %w",
					item.Name(),
					baseDir,
					err,
				)
			}

			// Check for actual parent directory traversal (CWE-23 mitigation)
			// This catches paths that escape baseDir like ".." or "../etc/passwd"
			// but allows legitimate directory names like "..suspicious" or "...triple"
			// which remain inside baseDir despite starting with dots.
			if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return fmt.Errorf(
					"'%s' is not inside of overlay working directory: %s",
					item.Name(),
					baseDir,
				)
			}

			wwlog.Verbose("removing overlays of deleted node: %s", item.Name())
			err = os.RemoveAll(cleanTarget)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// CleanTPMState removes the TPM state of deleted nodes.
func CleanTPMState() error {
	nodeDB, err := node.New()
	if err != nil {
		return err
	}
	nodes := nodeDB.ListAllNodes()
	store := tpm.DefaultStore()
	stateNodes, err := store.Nodes()
	if err != nil {
		return err
	}
	for _, n := range stateNodes {
		if !slices.Contains(nodes, n) {
			wwlog.Verbose("removing TPM state of deleted node: %s", n)
			if err := store.Delete(n); err != nil {
				return err
			}
		}
	}
	return nil
}
