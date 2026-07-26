package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"runtime"

	"github.com/Liam-Weitzel/Impasse/gfx"
	"github.com/Liam-Weitzel/Impasse/proto"
	"github.com/gdamore/tcell/v2"
	"github.com/veandco/go-sdl2/sdl"
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

func connectionParams() (address, token string, err error) {
	address, ok := os.LookupEnv("IMPASSE_CONNECTION")
	if !ok {
		return "", "", errors.New("'IMPASSE_CONNECTION' is missing")
	}
	token, ok = os.LookupEnv("IMPASSE_TOKEN")
	if !ok {
		return "", "", errors.New("'IMPASSE_TOKEN' is missing")
	}
	return address, token, nil
}

func main() {
	var (
		logFile         = flag.String("log", "", "Log file")
		idleDuration    = flag.Duration("idle", 0, "idle duration")
		sessionDuration = flag.Duration("duration", 0, "session duration")
		tiles           = flag.String("tiles", "",
			"PNG tile atlas, empty for the generated one")
	)
	flag.Parse()

	connection, token, err := connectionParams()
	check(err)

	var toMenu bool

	run := func() error {
		con, err := dial(connection)
		if err != nil {
			return err
		}
		defer con.close()

		// The map and our id come from the server, so this has to complete
		// before there is anything to draw.
		welcome, g, err := con.handshake(token)
		if err != nil {
			return err
		}

		return gfx.WrapScreen(func(screen tcell.Screen) error {
			return gfx.WrapWindow(func(window *sdl.Window) error {
				var err error
				toMenu, err = startClient(
					con, welcome, g,
					screen, window,
					(*idleDuration).Abs(),
					(*sessionDuration).Abs(),
					*tiles,
				)
				return err
			})
		})
	}

	check(logWrap(*logFile, run))

	// The SSH server spawned this process and only sees how it finished, so
	// the exit status is what tells it to put the menu back up.
	if toMenu {
		os.Exit(proto.ExitToMenu)
	}
}
