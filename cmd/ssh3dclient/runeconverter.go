// This is Free Software covered by the terms of the Apache 2.0 license.
// See LICENSE file for details.
// (c) 2018 by Sascha L. Teichmann
package main

import (
	"image"
	"math"
	"math/bits"
)

type bitsRune struct {
	pixel uint32
	r     rune
}

var bitmaps = []bitsRune{
	{0x00000000, '\u00a0'},

	// Block graphics

	// 0xffff0000, '\u2580',  // upper 1/2; redundant with inverse lower 1/2

	{0x0000000f, '\u2581'}, // lower 1/8
	{0x000000ff, '\u2582'}, // lower 1/4
	{0x00000fff, '\u2583'},
	{0x0000ffff, '\u2584'}, // lower 1/2
	{0x000fffff, '\u2585'},
	{0x00ffffff, '\u2586'}, // lower 3/4
	{0x0fffffff, '\u2587'},
	// 0xffffffff, '\u2588',  // full; redundant with inverse space

	{0xeeeeeeee, '\u258a'}, // left 3/4
	{0xcccccccc, '\u258c'}, // left 1/2
	{0x88888888, '\u258e'}, // left 1/4

	{0x0000cccc, '\u2596'}, // quadrant lower left
	{0x00003333, '\u2597'}, // quadrant lower right
	{0xcccc0000, '\u2598'}, // quadrant upper left
	// 0xccccffff, '\u2599',  // 3/4 redundant with inverse 1/4
	{0xcccc3333, '\u259a'}, // diagonal 1/2
	// 0xffffcccc, '\u259b',  // 3/4 redundant
	// 0xffff3333, '\u259c',  // 3/4 redundant
	{0x33330000, '\u259d'}, // quadrant upper right
	// 0x3333cccc, '\u259e',  // 3/4 redundant
	// 0x3333ffff, '\u259f',  // 3/4 redundant

	// Line drawing subset: no double lines, no complex light lines
	// Simple light lines duplicated because there is no center pixel int the 4x8 matrix

	{0x000ff000, '\u2501'}, // Heavy horizontal
	{0x66666666, '\u2503'}, // Heavy vertical

	{0x00077666, '\u250f'}, // Heavy down and right
	{0x000ee666, '\u2513'}, // Heavy down and left
	{0x66677000, '\u2517'}, // Heavy up and right
	{0x666ee000, '\u251b'}, // Heavy up and left

	{0x66677666, '\u2523'}, // Heavy vertical and right
	{0x666ee666, '\u252b'}, // Heavy vertical and left
	{0x000ff666, '\u2533'}, // Heavy down and horizontal
	{0x666ff000, '\u253b'}, // Heavy up and horizontal
	{0x666ff666, '\u254b'}, // Heavy cross

	{0x000cc000, '\u2578'}, // Bold horizontal left
	{0x00066000, '\u2579'}, // Bold horizontal up
	{0x00033000, '\u257a'}, // Bold horizontal right
	{0x00066000, '\u257b'}, // Bold horizontal down

	{0x06600660, '\u254f'}, // Heavy double dash vertical

	{0x000f0000, '\u2500'}, // Light horizontal
	{0x0000f000, '\u2500'}, //
	{0x44444444, '\u2502'}, // Light vertical
	{0x22222222, '\u2502'},

	{0x000e0000, '\u2574'}, // light left
	{0x0000e000, '\u2574'}, // light left
	{0x44440000, '\u2575'}, // light up
	{0x22220000, '\u2575'}, // light up
	{0x00030000, '\u2576'}, // light right
	{0x00003000, '\u2576'}, // light right
	{0x00004444, '\u2575'}, // light down
	{0x00002222, '\u2575'}, // light down

	// Misc technical

	{0x44444444, '\u23a2'}, // [ extension
	{0x22222222, '\u23a5'}, // ] extension

	//12345678
	{0x0f000000, '\u23ba'}, // Horizontal scanline 1
	{0x00f00000, '\u23bb'}, // Horizontal scanline 3
	{0x00000f00, '\u23bc'}, // Horizontal scanline 7
	{0x000000f0, '\u23bd'}, // Horizontal scanline 9

	// Geometrical shapes. Tricky because some of them are too wide.

	{0x00ffff00, '\u25fe'}, // Black medium small square

	{0x00066000, '\u25aa'}, // Black small square

	{0x11224488, '\u2571'}, // diagonals
	{0x88442211, '\u2572'},
	{0x99666699, '\u2573'},
	{0x000137f0, '\u25e2'}, // Triangles
	{0x0008cef0, '\u25e3'},
	{0x000fec80, '\u25e4'},
	{0x000f7310, '\u25e5'},
	/*
	 */
}

var shades = []rune{
	' ',
	'\u2591',
	'\u2592',
	'\u2593',
	'\u2588',
}

type RuneConverter struct {
	bitmaps   []bitsRune
	CodePoint rune

	BGColor [3]int
	FGColor [3]int
}

func NewRuneConverter(useGeoms bool) RuneConverter {
	var bms []bitsRune
	if useGeoms {
		bms = bitmaps
	} else {
		bms = bitmaps[:len(bitmaps)-9]
	}
	return RuneConverter{bitmaps: bms}
}

func (rc *RuneConverter) Extract(img *image.RGBA, x, y int) {
	p0 := img.PixOffset(x, y)

	var min, max [3]uint8

	for i := range min {
		min[i] = 255
	}

	for i := range rc.FGColor {
		rc.FGColor[i] = 0
	}

	for i := range rc.BGColor {
		rc.BGColor[i] = 0
	}

	data := img.Pix
	// Determine the minimum and maximum value for each color channel
	for y, pos := 0, p0; y < 8; y, pos = y+1, pos+img.Stride-4*4 {
		for x := 0; x < 4; x++ {
			for i := 0; i < 3; i, pos = i+1, pos+1 {
				if data[pos] < min[i] {
					min[i] = data[pos]
				}
				if data[pos] > max[i] {
					max[i] = data[pos]
				}

			}
			pos++ // alpha
		}
	}

	// Determine the color channel with the greatest range.

	var splitIndex int
	var bestSplit uint8

	for i := range min {
		if max[i]-min[i] > bestSplit {
			bestSplit = max[i] - min[i]
			splitIndex = i
		}
	}

	// We just split at the middle of the interval instead of computing the median.
	splitValue := min[splitIndex] + bestSplit/2

	var pixel uint32
	var fgCount int

	for y, pos := 0, p0; y < 8; y, pos = y+1, pos+img.Stride-4*4 {
		for x := 0; x < 4; x++ {
			pixel <<= 1
			var avg *[3]int
			if data[pos+splitIndex] > splitValue {
				avg = &rc.FGColor
				pixel |= 1
				fgCount++
			} else {
				avg = &rc.BGColor
			}
			for i := range *avg {
				avg[i] += int(data[pos])
				pos++
			}
			pos++
		}
	}
	bgCount := 8*4 - fgCount

	if bgCount > 0 {
		for i := range rc.BGColor {
			rc.BGColor[i] /= bgCount
		}
	}

	if fgCount > 0 {
		for i := range rc.FGColor {
			rc.FGColor[i] /= fgCount
		}
	}

	rc.CodePoint = 0
	bestDiff := math.MaxInt32
	invert := false

	for i := range rc.bitmaps {
		bm := &rc.bitmaps[i]
		if diff := bits.OnesCount32(bm.pixel ^ pixel); diff < bestDiff {
			bestDiff = diff
			rc.CodePoint = bm.r
			invert = false
		}
		if diff := bits.OnesCount32(^bm.pixel ^ pixel); diff < bestDiff {
			bestDiff = diff
			rc.CodePoint = bm.r
			invert = true
		}
	}
	// If the match is quite bad, use a shade image instead.
	if bestDiff > 10 {
		invert = false
		idx := fgCount * 5 / 32
		if idx >= len(shades) {
			idx = len(shades) - 1
		}
		rc.CodePoint = shades[idx]
	}

	// If we use an inverted character, we need to swap the colors.
	if invert {
		rc.FGColor, rc.BGColor = rc.BGColor, rc.FGColor
	}
}
