package thumbhash

import (
	"errors"
	"image"
	"image/draw"
	"math"
)

var (
	ErrInvalidHash = errors.New("invalid hash")
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

func Decode(hash Hash) (image.Image, error) {
	// Read the content of the hash
	if len(hash) < 5 {
		return nil, ErrInvalidHash
	}

	header24 := int(hash[0]) | int(hash[1])<<8 | int(hash[2])<<16

	lDC := float64(header24&63) / 63.0
	pDC := float64((header24>>6)&63)/31.5 - 1.0
	qDC := float64((header24>>12)&63)/31.5 - 1.0
	lScale := float64((header24>>18)&31) / 31.0
	hasAlphaBit := header24 >> 23
	hasAlpha := hasAlphaBit == 1

	header16 := int(hash[3]) | int(hash[4])<<8

	pScale := float64((header16>>3)&63) / 63.0
	qScale := float64((header16>>9)&63) / 63.0

	isLandscapeBit := header16 >> 15
	isLandscape := isLandscapeBit == 1

	var lx, ly int
	if isLandscape {
		if hasAlpha {
			lx = 5
		} else {
			lx = 7
		}
		ly = imax(3, int(header16&7))
	} else {
		lx = imax(3, int(header16&7))
		if hasAlpha {
			ly = 5
		} else {
			ly = 7
		}
	}

	aDC := 1.0
	aScale := 0.0
	if hasAlpha {
		if len(hash) < 6 {
			return nil, ErrInvalidHash
		}

		aDC = float64(hash[5]&15) / 15.0
		aScale = float64(hash[5]>>4) / 15.0
	}

	start := 5
	if hasAlpha {
		start = 6
	}

	idx := 0

	var err error
	decodeChannel := func(nx, ny int, scale float64) (ac []float64) {
		for cy := 0; cy < ny; cy++ {
			var cx int
			if cy == 0 {
				cx = 1
			}

			for ; cx*ny < nx*(ny-cy); cx++ {
				hidx := start + (idx / 2)
				if hidx >= len(hash) {
					err = ErrInvalidHash
					return nil
				}

				f := (float64((hash[hidx]>>((idx&1)*4))&15)/7.5 - 1.0) * scale
				ac = append(ac, f)
				idx++
			}
		}

		return
	}

	// Note the multiplication by a constant factor to increase saturation
	// since quantization tend to produce dull images.
	lAC := decodeChannel(lx, ly, lScale)
	pAC := decodeChannel(3, 3, pScale*1.25)
	qAC := decodeChannel(3, 3, qScale*1.25)

	var aAC []float64
	if hasAlpha {
		aAC = decodeChannel(5, 5, aScale)
	}

	if err != nil {
		return nil, err
	}

	// Prepare the image
	ratio := float64(lx) / float64(ly)

	var w, h int
	if ratio > 1.0 {
		w = 32
		h = iround(32.0 / ratio)
	} else {
		w = iround(32.0 * ratio)
		h = 32
	}

	wf := float64(w)
	hf := float64(h)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	data := img.Pix

	// Decode RGBA data
	idx = 0

	for y := 0; y < h; y++ {
		yf := float64(y)

		for x := 0; x < w; x++ {
			xf := float64(x)

			l := lDC
			p := pDC
			q := qDC
			a := aDC

			// Precompute coefficients
			n := 3
			if hasAlpha {
				n = 5
			}
			n = imax(lx, n)

			fx := make([]float64, n)
			for cx := 0; cx < n; cx++ {
				fx[cx] = math.Cos(math.Pi / wf * (xf + 0.5) * float64(cx))
			}

			n = 3
			if hasAlpha {
				n = 5
			}
			n = imax(ly, n)

			fy := make([]float64, n)
			for cy := 0; cy < n; cy++ {
				fy[cy] = math.Cos(math.Pi / hf * (yf + 0.5) * float64(cy))
			}

			// Decode L
			j := 0
			for cy := 0; cy < ly; cy++ {
				cx := 0
				if cy == 0 {
					cx = 1
				}

				fy2 := fy[cy] * 2.0
				for ; cx*ly < lx*(ly-cy); cx++ {
					l += lAC[j] * fx[cx] * fy2
					j++
				}
			}

			// Decode P and Q
			j = 0
			for cy := 0; cy < 3; cy++ {
				cx := 0
				if cy == 0 {
					cx = 1
				}

				fy2 := fy[cy] * 2.0
				for ; cx < 3-cy; cx++ {
					f := fx[cx] * fy2
					p += pAC[j] * f
					q += qAC[j] * f
					j++
				}
			}

			// Decode A
			if hasAlpha {
				j = 0
				for cy := 0; cy < 5; cy++ {
					cx := 0
					if cy == 0 {
						cx = 1
					}

					fy2 := fy[cy] * 2.0
					for ; cx < 5-cy; cx++ {
						a += aAC[j] * fx[cx] * fy2
						j++
					}
				}
			}

			// Convert to RGBA
			b := l - 2.0/3.0*p
			r := (3.0*l - b + q) / 2.0
			g := r - q

			af := math.Max(0.0, math.Min(1.0, a))

			data[idx] = byte(math.Max(0.0, math.Min(1.0, r)*255.0*af))
			data[idx+1] = byte(math.Max(0.0, math.Min(1.0, g)*255.0*af))
			data[idx+2] = byte(math.Max(0.0, math.Min(1.0, b)*255.0*af))
			data[idx+3] = byte(af * 255.0)

			idx += 4
		}
	}

	return img, nil
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
