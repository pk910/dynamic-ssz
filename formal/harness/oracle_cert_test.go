package harness

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// certOracle talks to ssz-oracle (formal/lean), the decision oracle over the
// in-house certified codecs. Besides the compiled certificates it builds a
// codec for any declaration submitted at run time (`T <name> <json>`,
// SszCert.Declare) from the proof-carrying combinators.
type certOracle struct {
	t  *testing.T
	wr *bufio.Writer
	rd *bufio.Reader
}

func startCertOracle(t *testing.T) *certOracle {
	path := os.Getenv("SSZ_ORACLE")
	if path == "" {
		path = filepath.Join("..", "lean", ".lake", "build", "bin", "ssz-oracle")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("ssz-oracle not built: %v", err)
	}
	cmd := exec.Command("bash", "-c", "ulimit -s unlimited 2>/dev/null; exec '"+path+"'")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start oracle: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return &certOracle{t: t, wr: bufio.NewWriterSize(stdin, 1<<20), rd: bufio.NewReaderSize(stdout, 1<<20)}
}

func (o *certOracle) ask(line string) string {
	if oracleTrace == "1" {
		fmt.Fprintln(os.Stderr, "cert-oracle<", line)
	}
	fmt.Fprintln(o.wr, line)
	if err := o.wr.Flush(); err != nil {
		o.t.Fatalf("write to oracle: %v", err)
	}
	type answer struct {
		text string
		err  error
	}
	done := make(chan answer, 1)
	go func() {
		text, err := o.rd.ReadString('\n')
		done <- answer{text, err}
	}()
	var ans answer
	select {
	case ans = <-done:
	case <-time.After(oracleAnswerTimeout):
		o.t.Fatalf("in-house oracle did not answer within %s to: %s", oracleAnswerTimeout, line)
	}
	if ans.err != nil {
		o.t.Fatalf("in-house oracle died on %q: %v", line, ans.err)
	}
	text := strings.TrimSpace(ans.text)
	if strings.HasPrefix(text, "ERR") || strings.HasPrefix(text, "MISMATCH") {
		o.t.Fatalf("in-house oracle answered %q for %q", text, line)
	}
	return text
}

// declare submits a declaration (CertJSON spelling) for this session.
func (o *certOracle) declare(name, declaration string) string {
	if strings.ContainsAny(declaration, " \t\r\n") {
		o.t.Fatalf("declaration of %s contains whitespace", name)
	}
	return o.ask("T " + name + " " + declaration)
}

// verdict is decode under its older name.
func (o *certOracle) verdict(name string, raw []byte) (bool, string) { return o.decode(name, raw) }

// decode answers the certified decoder's verdict and root for raw bytes.
func (o *certOracle) decode(name string, raw []byte) (bool, string) {
	ans := o.ask(name + " " + hexOrDash(raw))
	if root, ok := strings.CutPrefix(ans, "ACCEPT "); ok {
		return true, root
	}
	if ans == "REJECT" {
		return false, ""
	}
	o.t.Fatalf("unexpected in-house oracle answer %q", ans)
	return false, ""
}
