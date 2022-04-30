package gfx

import (
	"image"
	"runtime"
	"sync"

	"github.com/gdamore/tcell/v2"
)

func BlitRunes(s tcell.Screen, canvas *image.RGBA, geoms bool) {

	width, height := canvas.Rect.Dx(), canvas.Rect.Dy()

	var wg sync.WaitGroup

	rows := make(chan int)

	n := runtime.NumCPU()

	type cell struct {
		r  rune
		st tcell.Style
	}

	type rowRes struct {
		row   int
		cells []cell
	}

	leaky := make(chan []cell, n)

	alloc := func() []cell {
		select {
		case cells := <-leaky:
			return cells
		default:
			return make([]cell, width/4)
		}
	}

	free := func(cells []cell) {
		select {
		case leaky <- cells:
		default:
		}
	}

	converted := make(chan rowRes, n)

	done := make(chan struct{})

	go func() {
		defer close(done)
		for res := range converted {
			for j := range res.cells {
				cell := &res.cells[j]
				s.SetContent(j, res.row, cell.r, nil, cell.st)
			}
			free(res.cells)
		}
	}()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc := NewRuneConverter(geoms)
			for i := range rows {
				x, y := 0, i*8
				cells := alloc()
				for j := range cells {
					rc.Extract(canvas, x, y)
					st := tcell.StyleDefault.
						Foreground(tcell.NewRGBColor(
							int32(rc.FGColor[0]),
							int32(rc.FGColor[1]),
							int32(rc.FGColor[2]))).
						Background(tcell.NewRGBColor(
							int32(rc.BGColor[0]),
							int32(rc.BGColor[1]),
							int32(rc.BGColor[2])))
					cells[j] = cell{
						r:  rc.CodePoint,
						st: st,
					}
					x += 4
				}
				converted <- rowRes{row: i, cells: cells}
			}
		}()
	}

	for i, y := 0, 0; y < height; y, i = y+8, i+1 {
		rows <- i
	}

	close(rows)
	wg.Wait()
	close(converted)
	<-done
}
