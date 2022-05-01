package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
)

func loadScene(fname string) (*x3d.Scene, error) {

	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r io.Reader

	if strings.HasSuffix(strings.ToLower(fname), ".gz") {
		if r, err = gzip.NewReader(f); err != nil {
			return nil, err
		}
	} else {
		r = bufio.NewReader(f)
	}

	return x3d.ParseScene(r)
}

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

func main() {
	var (
		sceneFile = flag.String("scene", "scene.x3d.gz", "X3D scene to load")
		logFile   = flag.String("log", "", "Log file")
	)
	flag.Parse()

	scene, err := loadScene(*sceneFile)
	check(err)

	run := func() error {
		return gfx.WrapScreen(func(screen tcell.Screen) error {
			return gfx.WrapWindow(func(window *sdl.Window) error {
				directory := filepath.Dir(*sceneFile)
				return startClient(scene, directory, screen, window)
			})
		})
	}

	check(logWrap(*logFile, run))
}
