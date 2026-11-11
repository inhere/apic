// Command stub is a test double for exec authentication. Its behavior is a
// list of operations given as arguments, processed in order:
//
//	echoargs         print all arguments to stderr
//	count=FILE       append one line to FILE
//	info=FILE        write cwd and APIC_*/STUB_FOO environment to FILE
//	stdin=FILE       write "eof" or "blocked" to FILE depending on stdin
//	sleep=DURATION   sleep
//	grand=PIDFILE    start a grandchild (sleeps 20s), write its pid to PIDFILE
//	big              write 5 MiB to stdout
//	bigerr           write 5 MiB to stderr
//	token=TEXT       print TEXT (Go-quoted escapes allowed) to stdout
//	json=TEXT        print TEXT to stdout, unmodified
//	errline=TEXT     print TEXT to stderr
//	exit=N           exit with status N
//
// Unknown arguments are ignored.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {
	for _, a := range os.Args[1:] {
		op, val, _ := strings.Cut(a, "=")
		switch op {
		case "echoargs":
			fmt.Fprintln(os.Stderr, strings.Join(os.Args[1:], " "))
		case "count":
			f, _ := os.OpenFile(val, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			fmt.Fprintln(f, "x")
			f.Close()
		case "info":
			cwd, _ := os.Getwd()
			s := fmt.Sprintf("cwd=%s\nsite=%s\nenv=%s\nauth=%s\nfoo=%s\nparent=%s\n", cwd,
				os.Getenv("APIC_SITE"), os.Getenv("APIC_ENV"), os.Getenv("APIC_AUTH"), os.Getenv("STUB_FOO"), os.Getenv("STUB_PARENT"))
			_ = os.WriteFile(val, []byte(s), 0o600)
		case "stdin":
			done := make(chan string, 1)
			go func() { _, _ = io.Copy(io.Discard, os.Stdin); done <- "eof" }()
			res := "blocked"
			select {
			case res = <-done:
			case <-time.After(1500 * time.Millisecond):
			}
			_ = os.WriteFile(val, []byte(res), 0o600)
		case "sleep":
			d, _ := time.ParseDuration(val)
			time.Sleep(d)
		case "grand":
			c := exec.Command(os.Args[0], "sleep=20s")
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			if err := c.Start(); err == nil {
				_ = os.WriteFile(val, []byte(strconv.Itoa(c.Process.Pid)), 0o600)
			}
		case "big":
			chunk := []byte(strings.Repeat("a", 1<<16))
			for i := 0; i < 80; i++ {
				os.Stdout.Write(chunk)
			}
		case "bigerr":
			chunk := []byte(strings.Repeat("a", 1<<16))
			for i := 0; i < 80; i++ {
				os.Stderr.Write(chunk)
			}
		case "token":
			if u, err := strconv.Unquote(`"` + val + `"`); err == nil {
				val = u
			}
			fmt.Println(val)
		case "json":
			fmt.Print(val)
		case "errline":
			fmt.Fprintln(os.Stderr, val)
		case "exit":
			n, _ := strconv.Atoi(val)
			os.Exit(n)
		}
	}
}
