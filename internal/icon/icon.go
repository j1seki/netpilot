// Package icon renders the NetPilot application and system tray icons:
// a rounded obsidian/emerald badge with a twin-uplink convergence arrow ("NetPilot")
// in bright emerald/cyan when active, or cool zinc slate when on standby.
package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
)

func Icon(on bool) []byte {
	if runtime.GOOS == "windows" {
		return ICO(on)
	}
	return PNG(on)
}

func PNG(on bool) []byte {
	return PNGSize(on, 64)
}

// PNGSize renders the NetPilot icon at an arbitrary square resolution (e.g. 16, 32, 64, 128, 256, 512).
func PNGSize(on bool, size int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, draw(on, size))
	return b.Bytes()
}

func ICO(on bool) []byte {
	pngData := PNGSize(on, 64)
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, 1})
	b.Write([]byte{64, 64, 0, 0})
	_ = binary.Write(&b, binary.LittleEndian, [2]uint16{1, 32})
	_ = binary.Write(&b, binary.LittleEndian, [2]uint32{uint32(len(pngData)), 6 + 16})
	b.Write(pngData)
	return b.Bytes()
}

func draw(on bool, size int) *image.NRGBA {
	if size < 16 {
		size = 16
	}
	m := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / 32.0

	// Palette (normalized 0..32 coordinates)
	// Rounded squircle background with gradient + crisp border + dual-stream pilot arrow
	var topBg, botBg, borderCol, accentCol, fgCol [3]float64
	if on {
		topBg = [3]float64{12, 34, 29}       // deep emerald-zinc
		botBg = [3]float64{9, 18, 24}        // deep slate-cyan
		borderCol = [3]float64{16, 185, 129} // #10b981 emerald border
		accentCol = [3]float64{56, 189, 248} // #38bdf8 sky cyan accent
		fgCol = [3]float64{250, 250, 250}    // crisp white
	} else {
		topBg = [3]float64{30, 30, 36}       // zinc-800
		botBg = [3]float64{18, 18, 22}       // zinc-950
		borderCol = [3]float64{82, 82, 91}   // zinc-600
		accentCol = [3]float64{113, 113, 122}
		fgCol = [3]float64{161, 161, 170}
	}

	// Geometry in 32x32 space:
	// Two source nodes at bottom-left (9.0, 22.5) and bottom-right (23.0, 22.5)
	// converging into central hub (16.0, 14.5) and shooting upward to arrow tip (16.0, 6.5)
	// with pilot wings (11.5, 11.0) -> (16.0, 6.5) -> (20.5, 11.0)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			// 2x2 supersampling for ultra-sharp edges at all icon sizes
			var rSum, gSum, bSum, aSum float64
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					x := (float64(px) + (float64(sx)+0.5)*0.5) / scale
					y := (float64(py) + (float64(sy)+0.5)*0.5) / scale

					// Rounded squircle SDF centered at (16,16), half-size 13.8, corner radius 6.5
					sd := sdRoundedBox(x-16.0, y-16.0, 13.8, 13.8, 6.5)
					if sd > 0.6 {
						continue
					}
					bgAlpha := clamp01(0.5 - sd)

					// Vertical gradient factor
					t := clamp01((y - 2.0) / 28.0)
					cr := topBg[0]*(1-t) + botBg[0]*t
					cg := topBg[1]*(1-t) + botBg[1]*t
					cb := topBg[2]*(1-t) + botBg[2]*t

					// Border ring near sd == 0 (thickness ~1.35 in 32x32 space)
					borderDist := math.Abs(sd + 0.65)
					borderMask := clamp01(1.1 - borderDist)
					cr = cr*(1-borderMask*0.85) + borderCol[0]*(borderMask*0.85)
					cg = cg*(1-borderMask*0.85) + borderCol[1]*(borderMask*0.85)
					cb = cb*(1-borderMask*0.85) + borderCol[2]*(borderMask*0.85)

					// Left & right tributary streams (cyan/emerald accent)
					dLeft := segDist(x, y, 9.2, 22.5, 16.0, 15.2)
					dRight := segDist(x, y, 22.8, 22.5, 16.0, 15.2)
					dNodeL := math.Hypot(x-9.2, y-22.5) - 0.85
					dNodeR := math.Hypot(x-22.8, y-22.5) - 0.85
					streamDist := math.Min(math.Min(dLeft, dRight)-1.25, math.Min(dNodeL, dNodeR)-1.25)
					streamMask := clamp01(0.55 - streamDist)
					if streamMask > 0 {
						cr = cr*(1-streamMask) + accentCol[0]*streamMask
						cg = cg*(1-streamMask) + accentCol[1]*streamMask
						cb = cb*(1-streamMask) + accentCol[2]*streamMask
					}

					// Central unified trunk + upward pilot chevron (crisp white/bright)
					dStem := segDist(x, y, 16.0, 24.0, 16.0, 6.8) - 1.35
					dWingL := segDist(x, y, 11.2, 11.6, 16.0, 6.8) - 1.35
					dWingR := segDist(x, y, 20.8, 11.6, 16.0, 6.8) - 1.35
					dHub := math.Hypot(x-16.0, y-15.2) - 2.1
					fgDist := math.Min(math.Min(dStem, dHub), math.Min(dWingL, dWingR))
					fgMask := clamp01(0.55 - fgDist)
					if fgMask > 0 {
						cr = cr*(1-fgMask) + fgCol[0]*fgMask
						cg = cg*(1-fgMask) + fgCol[1]*fgMask
						cb = cb*(1-fgMask) + fgCol[2]*fgMask
					}

					rSum += cr * bgAlpha
					gSum += cg * bgAlpha
					bSum += cb * bgAlpha
					aSum += bgAlpha
				}
			}

			if aSum > 0 {
				a := aSum / 4.0
				r := (rSum / 4.0) / a
				g := (gSum / 4.0) / a
				b := (bSum / 4.0) / a
				m.SetNRGBA(px, py, color.NRGBA{
					R: uint8(math.Min(255, math.Max(0, math.Round(r)))),
					G: uint8(math.Min(255, math.Max(0, math.Round(g)))),
					B: uint8(math.Min(255, math.Max(0, math.Round(b)))),
					A: uint8(math.Min(255, math.Max(0, math.Round(a*255)))),
				})
			}
		}
	}
	return m
}

func sdRoundedBox(px, py, bx, by, r float64) float64 {
	qx := math.Abs(px) - bx + r
	qy := math.Abs(py) - by + r
	out := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	in := math.Min(math.Max(qx, qy), 0)
	return out + in - r
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}
