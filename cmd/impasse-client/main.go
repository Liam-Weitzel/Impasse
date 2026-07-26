package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"runtime"

	"github.com/Liam-Weitzel/Impasse/gfx"
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
			"PNG tile atlas, empty for the theme's generated one")
		themeName = flag.String("theme", "gritty", "look: gritty or matrix")
		modelName = flag.String("model", "pylon",
			"player marker: sphere, cube, prism, diamond or pylon")
	)
	flag.Parse()

	connection, token, err := connectionParams()
	check(err)

	th, err := themeByName(*themeName)
	check(err)

	model, err := parseModel(*modelName)
	check(err)

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
				return startClient(
					con, welcome, g,
					screen, window,
					(*idleDuration).Abs(),
					(*sessionDuration).Abs(),
					*tiles,
					th,
					model,
				)
			})
		})
	}

	check(logWrap(*logFile, run))
}
