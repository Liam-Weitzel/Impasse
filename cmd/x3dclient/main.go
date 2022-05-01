package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
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

func main() {
	var (
		sceneFile = flag.String("scene", "scene.x3d.gz", "X3D scene to load")
		logFile   = flag.String("log", "", "Log file")
	)
	flag.Parse()

	scene, err := loadScene(*sceneFile)
	check(err)

	wrap := func(fn func()) func() { return fn }

	if *logFile != "" {
		logF, err := os.OpenFile(
			*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY,
			0666)
		check(err)
		old := log.Writer()
		log.SetOutput(logF)
		wrap = func(fn func()) func() {
			return func() {
				defer func() {
					log.SetOutput(old)
					logF.Close()
				}()
				fn()
			}
		}
	}

	run := wrap(func() {
		err = gfx.WrapScreen(func(screen tcell.Screen) error {
			return gfx.WrapWindow(func(window *sdl.Window) error {
				directory := filepath.Dir(*sceneFile)
				return startClient(scene, directory, screen, window)
			})
		})
	})

	sdl.Main(run)

	check(err)
}
