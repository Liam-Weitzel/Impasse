package gfx

import (
	"image"
	"runtime"
	"sync"

	"github.com/gdamore/tcell/v2"
)

func WriteString(sc tcell.Screen, x, y int, s string, st tcell.Style) {
	for _, r := range s {
		if r != ' ' {
			sc.SetContent(x, y, r, nil, st)
		}
		x++
	}
}

type (
	cell struct {
		r  rune
		st tcell.Style
	}

	rowRes struct {
		row   int
		cells []cell
	}

	leaky chan []cell
)

func (l leaky) alloc(n int) []cell {
	select {
	case cells := <-l:
		return cells
	default:
		return make([]cell, n)
	}
}

func (l leaky) free(cells []cell) {
	select {
	case l <- cells:
	default:
	}
}

func BlitRunes(s tcell.Screen, img *image.RGBA, geoms bool) {

	sWidth, sHeight := s.Size()

	var wg sync.WaitGroup

	rows := make(chan int)

	n := runtime.NumCPU()

	pool := make(leaky, n)

	converted := make(chan rowRes, n)

	done := make(chan struct{})

	go func() {
		defer close(done)
		for res := range converted {
			for x := range res.cells {
				cell := &res.cells[x]
				s.SetContent(x, res.row, cell.r, nil, cell.st)
			}
			pool.free(res.cells)
		}
	}()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc := NewRuneConverter(
				img.Rect.Dx(), img.Rect.Dy(),
				sWidth, sHeight,
				geoms)

			for y := range rows {
				cells := pool.alloc(sWidth)
				for x := range cells {
					rc.Extract(img.Pix, x, y)
					st := tcell.StyleDefault.
						Foreground(tcell.NewRGBColor(
							rc.FGColor[0],
							rc.FGColor[1],
							rc.FGColor[2])).
						Background(tcell.NewRGBColor(
							rc.BGColor[0],
							rc.BGColor[1],
							rc.BGColor[2]))
					cells[x] = cell{
						r:  rc.CodePoint,
						st: st,
					}
				}
				converted <- rowRes{row: y, cells: cells}
			}
		}()
	}

	for i := 0; i < sHeight; i++ {
		rows <- i
	}

	close(rows)
	wg.Wait()
	close(converted)
	<-done
}
