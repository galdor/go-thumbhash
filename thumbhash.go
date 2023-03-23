package thumbhash

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

type Hash = []byte

// Hash binary representation:
//
// L DC:        6 bit
// P DC:        6 bit
// L scale:     5 bit
// HasAlpha:    1 bit
//
// L count:     3 bit
// P scale:     6 bit
// Q scale:     6 bit
// IsLandscape: 1 bit
//
// If HasAlpha:
// A DC:        4 bit
// A scale:     4 bit
//
// L AC:        4 bit each
// P AC:        4 bit each
// Q AC:        4 bit each
//
// If HasAlpha:
// A AC:        4 bit each

func Encode(img image.Image) Hash {
	// Extract image data
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	w, h := bounds.Max.X, bounds.Max.Y
	nbPixels := w * h
	isLandscape := 0
	if w > h {
		isLandscape = 1
	}

	data := rgba.Pix

	// Compute the average value of each color channel
	var avgR, avgG, avgB, avgA float64

	for i := 0; i < nbPixels; i++ {
		a := float64(data[i*4+3]) / 255.0

		avgR += a / 255.0 * float64(data[i*4])
		avgG += a / 255.0 * float64(data[i*4+1])
		avgB += a / 255.0 * float64(data[i*4+2])
		avgA += a
	}

	if avgA > 0.0 {
		avgR /= avgA
		avgG /= avgA
		avgB /= avgA
	}

	// Convert image data from RGBA to LPQA
	lChannel := make([]float64, nbPixels) // luminance
	pChannel := make([]float64, nbPixels) // yellow - blue
	qChannel := make([]float64, nbPixels) // red - green
	aChannel := make([]float64, nbPixels) // alpha

	hasAlpha := avgA < float64(nbPixels)
	var lLimit float64
	if hasAlpha {
		lLimit = 5.0
	} else {
		lLimit = 7.0
	}

	wf := float64(w)
	hf := float64(h)
	maxWH := math.Max(wf, hf)

	lx := imax(1, iround((lLimit*wf)/maxWH))
	ly := imax(1, iround((lLimit*hf)/maxWH))

	for i := 0; i < nbPixels; i++ {
		a := float64(data[i*4+3]) / 255.0

		r := avgR*(1.0-a) + a/255.0*float64(data[i*4])
		g := avgG*(1.0-a) + a/255.0*float64(data[i*4+1])
		b := avgB*(1.0-a) + a/255.0*float64(data[i*4+2])

		lChannel[i] = (r + g + b) / 3.0
		pChannel[i] = (r+g)/2.0 - b
		qChannel[i] = r - g
		aChannel[i] = a
	}

	// Encode LPQA data using a DCT
	encodeChannel := func(channel []float64, nx, ny int) (dc float64, ac []float64, scale float64) {
		fx := make([]float64, w)

		for cy := 0; cy < ny; cy++ {
			cyf := float64(cy)

			for cx := 0; cx*ny < nx*(ny-cy); cx++ {
				cxf := float64(cx)
				f := 0.0

				for x := 0; x < w; x++ {
					fx[x] = math.Cos(math.Pi / wf * cxf * (float64(x) + 0.5))
				}

				for y := 0; y < h; y++ {
					fy := math.Cos(math.Pi / hf * cyf * (float64(y) + 0.5))

					for x := 0; x < w; x++ {
						f += channel[x+y*w] * fx[x] * fy
					}
				}

				f /= float64(nbPixels)

				if cx > 0 || cy > 0 {
					ac = append(ac, f)
					scale = math.Max(scale, math.Abs(f))
				} else {
					dc = f
				}
			}
		}

		if scale > 0.0 {
			for i := 0; i < len(ac); i++ {
				ac[i] = 0.5 + 0.5/scale*ac[i]
			}
		}

		return
	}

	lDC, lAC, lScale := encodeChannel(lChannel, imax(lx, 3), imax(ly, 3))
	pDC, pAC, pScale := encodeChannel(pChannel, 3, 3)
	qDC, qAC, qScale := encodeChannel(qChannel, 3, 3)

	var aDC, aScale float64
	var aAC []float64
	if hasAlpha {
		aDC, aAC, aScale = encodeChannel(aChannel, 5, 5)
	}

	// Create the hash
	nbAC := len(lAC) + len(pAC) + len(qAC)
	if hasAlpha {
		nbAC += len(aAC)
	}
	hashSize := 3 + 2 + (nbAC+1)/2
	if hasAlpha {
		hashSize += 1
	}

	hash := make(Hash, hashSize)

	header24 := iround(63.0 * lDC)
	header24 |= iround(31.5+31.5*pDC) << 6
	header24 |= iround(31.5+31.5*qDC) << 12
	header24 |= iround(31.0*lScale) << 18
	if hasAlpha {
		header24 |= 1 << 23
	}

	hash[0] = byte(header24)
	hash[1] = byte(header24 >> 8)
	hash[2] = byte(header24 >> 16)

	header16 := lx
	if isLandscape == 1 {
		header16 = ly
	}
	header16 |= iround(63.0*pScale) << 3
	header16 |= iround(63.0*qScale) << 9
	header16 |= isLandscape << 15

	hash[3] = byte(header16)
	hash[4] = byte(header16 >> 8)

	if hasAlpha {
		hash[5] = byte(iround(15.0*aDC) | iround(15.0*aScale)<<4)
	}

	acs := [][]float64{lAC, pAC, qAC}
	if hasAlpha {
		acs = append(acs, aAC)
	}

	start := 5
	if hasAlpha {
		start = 6
	}

	idx := 0

	for i := 0; i < len(acs); i++ {
		ac := acs[i]
		for j := 0; j < len(ac); j++ {
			f := ac[j]

			hash[start+(idx/2)] |= byte(iround(15.0*f) << ((idx & 1) * 4))
			idx += 1
		}
	}

	return hash
}

func Decode(Hash) (image.Image, error) {
	// TODO
	return nil, nil
}

func iround(x float64) int {
	return int(math.Round(x))
}

func imax(x, y int) int {
	if x >= y {
		return x
	}

	return y
}
