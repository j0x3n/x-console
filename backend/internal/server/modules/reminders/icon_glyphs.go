package reminders

import (
	"image"
	"image/color"
	"math"
)

func customGlyph(g string) bool {
	switch g {
	case "book", "moon", "dumbbell", "apple", "salad", "bike", "swim", "meditate", "tooth", "guitar", "pen":
		return true
	}
	return false
}

type point struct{ x, y float64 }
type circle struct {
	x, y, r float64
	filled  bool
}
type glyphSpec struct {
	lines   [][]point
	circles []circle
}

func drawGlyph(name string) *image.RGBA {
	s := glyphSpec{}
	line := func(p ...point) { s.lines = append(s.lines, p) }
	circ := func(x, y, r float64, filled bool) { s.circles = append(s.circles, circle{x, y, r, filled}) }
	switch name {
	case "book":
		line(point{96, 62}, point{60, 52}, point{45, 57}, point{45, 130}, point{61, 125}, point{96, 136}, point{132, 125}, point{147, 130}, point{147, 57}, point{132, 52}, point{96, 62}, point{96, 136})
		line(point{60, 77}, point{79, 82})
		line(point{60, 95}, point{79, 100})
		line(point{113, 82}, point{133, 77})
		line(point{113, 100}, point{133, 95})
	case "moon":
		// A crescent is a disk with the offset inner disk subtracted below.
	case "dumbbell":
		line(point{55, 96}, point{137, 96})
		line(point{54, 68}, point{54, 124})
		line(point{69, 58}, point{69, 134})
		line(point{123, 58}, point{123, 134})
		line(point{138, 68}, point{138, 124})
	case "apple":
		line(point{96, 73}, point{81, 61}, point{62, 63}, point{47, 82}, point{45, 105}, point{57, 127}, point{78, 139}, point{96, 133}, point{115, 139}, point{135, 127}, point{147, 105}, point{145, 82}, point{130, 63}, point{111, 61}, point{96, 73})
		line(point{96, 73}, point{99, 49}, point{116, 40})
		line(point{100, 51}, point{84, 42}, point{73, 44})
	case "salad":
		line(point{42, 88}, point{150, 88}, point{140, 119}, point{122, 139}, point{70, 139}, point{52, 119}, point{42, 88})
		circ(68, 75, 18, false)
		circ(99, 66, 22, false)
		circ(127, 75, 18, false)
	case "bike":
		circ(54, 122, 26, false)
		circ(138, 122, 26, false)
		line(point{54, 122}, point{78, 80}, point{108, 122}, point{54, 122})
		line(point{78, 80}, point{126, 80}, point{108, 122})
		line(point{138, 122}, point{124, 57}, point{111, 57})
		line(point{71, 70}, point{87, 70})
	case "swim":
		circ(126, 68, 12, false)
		line(point{74, 99}, point{97, 77}, point{70, 55}, point{46, 74})
		line(point{97, 77}, point{114, 100})
		line(point{38, 122}, point{52, 114}, point{66, 122}, point{80, 114}, point{94, 122}, point{108, 114}, point{122, 122}, point{136, 114}, point{150, 122})
	case "meditate":
		circ(96, 51, 13, false)
		line(point{78, 83}, point{96, 74}, point{114, 83})
		line(point{78, 83}, point{62, 108}, point{43, 102})
		line(point{114, 83}, point{130, 108}, point{149, 102})
		line(point{96, 74}, point{96, 115})
		line(point{96, 115}, point{61, 139}, point{43, 132}, point{63, 119}, point{127, 119}, point{149, 132}, point{131, 139}, point{96, 115})
	case "tooth":
		line(point{96, 57}, point{74, 46}, point{52, 54}, point{44, 74}, point{51, 97}, point{57, 124}, point{66, 144}, point{78, 146}, point{85, 119}, point{96, 112}, point{107, 119}, point{114, 146}, point{126, 144}, point{135, 124}, point{141, 97}, point{148, 74}, point{140, 54}, point{118, 46}, point{96, 57})
	case "guitar":
		circ(70, 125, 27, false)
		circ(92, 101, 23, false)
		line(point{99, 91}, point{139, 47}, point{149, 55}, point{111, 100})
		circ(83, 112, 8, false)
		line(point{61, 134}, point{73, 122})
		line(point{135, 44}, point{147, 40}, point{154, 53}, point{149, 62})
	case "pen":
		line(point{49, 142}, point{57, 112}, point{120, 49}, point{144, 73}, point{81, 136}, point{49, 142})
		line(point{111, 58}, point{135, 82})
		line(point{57, 112}, point{81, 136})
	}
	bg := color.RGBA{75, 123, 216, 255}
	if name == "apple" || name == "salad" {
		bg = color.RGBA{58, 155, 91, 255}
	}
	if name == "moon" {
		bg = color.RGBA{107, 91, 210, 255}
	}
	out := image.NewRGBA(image.Rect(0, 0, 192, 192))
	// Four samples per pixel soften the stroke and rounded background edges.
	for y := 0; y < 192; y++ {
		for x := 0; x < 192; x++ {
			var rr, gg, bb, aa int
			for _, dy := range []float64{.25, .75} {
				for _, dx := range []float64{.25, .75} {
					px, py := float64(x)+dx, float64(y)+dy
					cx, cy := math.Max(44, math.Min(148, px)), math.Max(44, math.Min(148, py))
					if math.Hypot(px-cx, py-cy) > 44 {
						continue
					}
					white := false
					if name == "moon" {
						white = math.Hypot(px-91, py-96) < 51 && math.Hypot(px-116, py-77) > 44
					}
					for _, l := range s.lines {
						for i := 1; i < len(l); i++ {
							if segmentDistance(px, py, l[i-1], l[i]) <= 4.7 {
								white = true
							}
						}
					}
					for _, c := range s.circles {
						d := math.Hypot(px-c.x, py-c.y)
						if c.filled && d <= c.r || !c.filled && math.Abs(d-c.r) <= 4.7 {
							white = true
						}
					}
					c := bg
					if white {
						c = color.RGBA{255, 255, 255, 255}
					}
					rr += int(c.R)
					gg += int(c.G)
					bb += int(c.B)
					aa += 255
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(rr / 4), uint8(gg / 4), uint8(bb / 4), uint8(aa / 4)})
		}
	}
	return out
}

func segmentDistance(x, y float64, a, b point) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	t := ((x-a.x)*dx + (y-a.y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(x-a.x-t*dx, y-a.y-t*dy)
}
