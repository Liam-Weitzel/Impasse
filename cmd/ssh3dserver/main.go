package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
)

type handler struct {
	renderer string
	args     []string
}

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}

func (h *handler) sshHandle(s ssh.Session) {

	ptyReq, winCh, isPty := s.Pty()
	if !isPty {
		io.WriteString(s, "non-interactive terminals are not supported\n")
		s.Exit(1)
		return
	}

	cmdCtx, cancelCmd := context.WithCancel(s.Context())
	defer cancelCmd()

	cmd := exec.CommandContext(
		cmdCtx, h.renderer,
		h.args...)

	cmd.Env = append(s.Environ(), fmt.Sprintf("TERM=%s", ptyReq.Term))

	f, err := pty.Start(cmd)
	if err != nil {
		io.WriteString(s, fmt.Sprintf("failed to initialize pseudo-terminal: %s\n", err))
		s.Exit(1)
		return
	}
	defer f.Close()

	go func() {
		for win := range winCh {
			setWinsize(f, win.Width, win.Height)
		}
	}()

	go func() {
		io.Copy(f, s)
	}()
	io.Copy(s, f)

	f.Close()
	cmd.Wait()
}

func main() {

	port := flag.Int("port", 2222, "ssh server port")
	renderer := flag.String("renderer", "ssh3dclient", "path to renderer")

	flag.Parse()

	h := handler{
		renderer: *renderer,
		args:     flag.Args(),
	}

	s := &ssh.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: h.sshHandle,
	}

	if err := s.ListenAndServe(); err != nil {
		log.Printf("error: %v\n", err)
	}
}
