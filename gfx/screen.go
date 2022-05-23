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

func BlitRunes(s tcell.Screen, canvas *image.RGBA, geoms bool) {

	cw, ch := canvas.Rect.Dx(), canvas.Rect.Dy()
	sw, sh := s.Size()

	if cw == sw*4 && ch == 8*sh {
		blitRunesFit(s, canvas, geoms)
	} else {
		blitRunesInterpolate(s, canvas, geoms)
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

func blitRunesInterpolate(s tcell.Screen, canvas *image.RGBA, geoms bool) {

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
			for j := range res.cells {
				cell := &res.cells[j]
				s.SetContent(j, res.row, cell.r, nil, cell.st)
			}
			pool.free(res.cells)
		}
	}()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc := NewRuneConverter(geoms)
			for i := range rows {
				y := i
				cells := pool.alloc(sWidth)
				for j := range cells {
					rc.ExtractInterpol(canvas, sWidth, sHeight, j, y)
					st := tcell.StyleDefault.
						Foreground(tcell.NewRGBColor(
							rc.FGColor[0],
							rc.FGColor[1],
							rc.FGColor[2])).
						Background(tcell.NewRGBColor(
							rc.BGColor[0],
							rc.BGColor[1],
							rc.BGColor[2]))
					cells[j] = cell{
						r:  rc.CodePoint,
						st: st,
					}
				}
				converted <- rowRes{row: i, cells: cells}
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

func blitRunesFit(s tcell.Screen, canvas *image.RGBA, geoms bool) {

	width, height := s.Size()

	var wg sync.WaitGroup

	rows := make(chan int)

	n := runtime.NumCPU()

	pool := make(leaky, n)

	converted := make(chan rowRes, n)

	done := make(chan struct{})

	go func() {
		defer close(done)
		for res := range converted {
			for j := range res.cells {
				cell := &res.cells[j]
				s.SetContent(j, res.row, cell.r, nil, cell.st)
			}
			pool.free(res.cells)
		}
	}()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc := NewRuneConverter(geoms)
			for i := range rows {
				x, y := 0, i*8
				cells := pool.alloc(width)
				for j := range cells {
					rc.Extract(canvas, x, y)
					st := tcell.StyleDefault.
						Foreground(tcell.NewRGBColor(
							rc.FGColor[0],
							rc.FGColor[1],
							rc.FGColor[2])).
						Background(tcell.NewRGBColor(
							rc.BGColor[0],
							rc.BGColor[1],
							rc.BGColor[2]))
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

	for i := 0; i < height; i++ {
		rows <- i
	}

	close(rows)
	wg.Wait()
	close(converted)
	<-done
}
