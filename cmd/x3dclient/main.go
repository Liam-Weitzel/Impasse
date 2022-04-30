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
	sceneFile := flag.String("scene", "scene.x3d.gz", "X3D scene to load")
	flag.Parse()

	scene, err := loadScene(*sceneFile)
	check(err)
	directory := filepath.Dir(*sceneFile)

	var client *client
	client, err = newClient(scene, directory)
	check(err)

	check(client.run())
}
