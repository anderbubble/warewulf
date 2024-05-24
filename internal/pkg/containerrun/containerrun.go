// Run or exec into a container, must be extra as Build
// calls RunContainer
package containerrun

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"syscall"

	"github.com/pkg/errors"

	"github.com/warewulf/warewulf/internal/pkg/container"
	"github.com/warewulf/warewulf/internal/pkg/kernel"
	"github.com/warewulf/warewulf/internal/pkg/util"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

const (
	ExcludesFileName = "./etc/warewulf/excludes"
	TriggerDracut    = "/usr/lib/dracut/modules.d/90wwinit/"
	DracutBin        = "/usr/bin/dracut"
)

func Build(name string, buildForce bool) error {

	rootfsPath := container.RootFsDir(name)
	imagePath := container.ImageFile(name)

	if !container.ValidSource(name) {
		return errors.Errorf("Container does not exist: %s", name)
	}

	if !buildForce {
		wwlog.Debug("Checking if there have been any updates to the VNFS directory")
		if util.PathIsNewer(rootfsPath, imagePath) {
			wwlog.Info("Skipping (VNFS is current)")
			return nil
		}
	}

	if util.IsFile(path.Join(rootfsPath, DracutBin)) &&
		util.IsDir(path.Join(rootfsPath, TriggerDracut)) {
		wwlog.Debug("dracut build triggered")
		_, version, err := kernel.FindKernel(rootfsPath)
		if err == nil {
			err := RunContainedCmd("", []string{name,
				DracutBin, "--", "--force", "--no-hostonly", "--add", "wwinit", "--kver", version})
			if err != nil {
				// only warn if dracut fails
				wwlog.Warn("dracut failed with: %s", err)
			}
		} else {
			wwlog.Warn("couldn't find kernel for %s, dracut couldn't be run")
		}
	}
	ignore := []string{}
	excludes_file := path.Join(rootfsPath, ExcludesFileName)
	if util.IsFile(excludes_file) {
		var err error
		ignore, err = util.ReadFile(excludes_file)
		if err != nil {
			return errors.Wrapf(err, "Failed creating directory: %s", imagePath)
		}
	}

	err := util.BuildFsImage(
		"VNFS container "+name,
		rootfsPath,
		imagePath,
		[]string{"*"},
		ignore,
		// ignore cross-device files
		true,
		"newc")

	return err
}

/*
fork off a process with a new PID space
*/
func RunContainedCmd(tempDir string, args []string) (err error) {
	if tempDir == "" {
		tempDir, err = os.MkdirTemp(os.TempDir(), "overlay")
		if err != nil {
			wwlog.Warn("couldn't create temp dir for overlay", err)
		}
		defer func() {
			err = os.RemoveAll(tempDir)
			if err != nil {
				wwlog.Warn("Couldn't remove temp dir for ephermal mounts:", err)
			}
		}()
	}
	logStr := fmt.Sprint(wwlog.GetLogLevel())
	wwlog.Verbose("Running contained command: %s", args[1:])
	c := exec.Command("/proc/self/exe", append([]string{"--loglevel", logStr, "--tempdir", tempDir, "container", "exec", "__child"}, args...)...)

	c.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		fmt.Printf("Command exited non-zero, not rebuilding/updating VNFS image\n")
		// defer is not called before os.Exit(0)
		err = os.RemoveAll(tempDir)
		if err != nil {
			wwlog.Warn("Couldn't remove temp dir for ephermal mounts:", err)
		}
		os.Exit(0)
	}
	return nil
}
func Duplicate(name string, destination string) error {
	fullPathImageSource := container.RootFsDir(name)

	wwlog.Info("Copying sources...")
	err := container.ImportDirectory(fullPathImageSource, destination)

	if err != nil {
		return err
	}
	wwlog.Info("Building container: %s", destination)
	err = Build(destination, true)
	if err != nil {
		return err
	}
	return nil
}
