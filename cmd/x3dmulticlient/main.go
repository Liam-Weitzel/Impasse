package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"runtime"

	"github.com/gdamore/tcell/v2"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
)

func check(err error) {
	if err != nil {
		log.Fatalf("error: %v\n", err)
	}
}

func init() {
	runtime.LockOSThread()
}

func logWrap(fname string, fn func() error) (err error) {
	if fname == "" {
		return fn()
	}
	var (
		old  = log.Writer()
		logF *os.File
	)
	if logF, err = os.OpenFile(
		fname, os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0666,
	); err != nil {
		return
	}
	defer func() {
		log.SetOutput(old)
		if err2 := logF.Close(); err == nil {
			err = err2
		}
	}()
	log.SetOutput(logF)
	err = fn()
	return
}

func connectionParam() (string, error) {
	connection, ok := os.LookupEnv("SSH3D_CONNECTION")
	if !ok {
		return "", errors.New("'SSH3D_CONNECTION' is missing")
	}
	return connection, nil
}

func main() {
	var (
		logFile         = flag.String("log", "", "Log file")
		idleDuration    = flag.Duration("idle", 0, "idle duration")
		sessionDuration = flag.Duration("duration", 0, "session duration")
	)
	flag.Parse()

	connection, err := connectionParam()
	check(err)

	run := func() error {
		con, err := dial(connection)
		if err != nil {
			return err
		}
		defer con.close()

		// The map and our id come from the server, so this has to complete
		// before there is anything to draw.
		welcome, g, err := con.handshake()
		if err != nil {
			return err
		}

		return gfx.WrapScreen(func(screen tcell.Screen) error {
			return gfx.WrapWindow(func(window *sdl.Window) error {
				return startClient(
					con, welcome, g,
					screen, window,
					(*idleDuration).Abs(),
					(*sessionDuration).Abs(),
				)
			})
		})
	}

	check(logWrap(*logFile, run))
}
