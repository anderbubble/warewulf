//go:build tpmsim

package tpmtest

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/go-attestation/attest"
	"github.com/google/go-tpm/legacy/tpm2"
	"github.com/google/go-tpm/tpmutil"
)

// Swtpm is a software TPM 2.0 run by swtpm, for tests that exercise real
// TPM commands. swtpm has no firmware, so it has no measurement log; Extend
// measures into its PCRs and records a matching log.
type Swtpm struct {
	addr string

	mu  sync.Mutex
	log *EventLog
}

// StartSwtpm runs swtpm for the duration of t. If swtpm is not installed, t
// is skipped, or fails if WW_REQUIRE_SWTPM is set (as in CI).
func StartSwtpm(t testing.TB) *Swtpm {
	t.Helper()
	bin, err := exec.LookPath("swtpm")
	if err != nil {
		if os.Getenv("WW_REQUIRE_SWTPM") != "" {
			t.Fatal("swtpm is not installed")
		}
		t.Skip("swtpm is not installed")
	}
	ports := freePorts(t, 2)
	cmd := exec.Command(bin, "socket", "--tpm2",
		"--tpmstate", "dir="+t.TempDir(),
		"--server", "type=tcp,bindaddr=127.0.0.1,port="+strconv.Itoa(ports[0]),
		"--ctrl", "type=tcp,bindaddr=127.0.0.1,port="+strconv.Itoa(ports[1]),
		"--flags", "not-need-init,startup-clear")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting swtpm: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	s := &Swtpm{addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[0])), log: NewEventLog()}
	deadline := time.Now().Add(10 * time.Second)
	for {
		c, err := net.Dial("tcp", s.addr)
		if err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("swtpm did not start: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return s
}

func freePorts(t testing.TB, n int) []int {
	var ports []int
	for range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = l.Close() }()
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	return ports
}

// Open returns go-attestation's handle on the TPM, as attest.OpenTPM would
// for a real one.
func (s *Swtpm) Open() (*attest.TPM, error) {
	c, err := s.channel()
	if err != nil {
		return nil, err
	}
	return attest.OpenTPM(&attest.OpenConfig{CommandChannel: c})
}

// Extend measures data into pcr and logs it.
func (s *Swtpm) Extend(pcr int, eventType uint32, data []byte) error {
	c, err := s.channel()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	digest := sha256.Sum256(data)
	if err := tpm2.PCRExtend(c, tpmutil.Handle(pcr), tpm2.AlgSHA256, digest[:], ""); err != nil {
		return fmt.Errorf("extending PCR %d: %w", pcr, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.Add(pcr, eventType, digest[:], data)
	return nil
}

func (s *Swtpm) channel() (*swtpmChannel, error) {
	c, err := net.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	return &swtpmChannel{Conn: c, s: s}, nil
}

// swtpmChannel carries raw TPM commands to swtpm's server port. Each Read
// returns one whole response, as go-tpm expects of a TPM device.
type swtpmChannel struct {
	net.Conn
	s       *Swtpm
	pending []byte
}

func (c *swtpmChannel) Read(b []byte) (int, error) {
	if len(c.pending) == 0 {
		header := make([]byte, 10) // tag, size, response code
		if _, err := io.ReadFull(c.Conn, header); err != nil {
			return 0, err
		}
		size := binary.BigEndian.Uint32(header[2:6])
		if size < 10 {
			return 0, fmt.Errorf("short TPM response (%d bytes)", size)
		}
		resp := make([]byte, size)
		copy(resp, header)
		if _, err := io.ReadFull(c.Conn, resp[10:]); err != nil {
			return 0, err
		}
		c.pending = resp
	}
	n := copy(b, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func (c *swtpmChannel) MeasurementLog() ([]byte, error) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	return c.s.log.Bytes(), nil
}
