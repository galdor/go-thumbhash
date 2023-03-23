package thumbhash

import (
	"image"
	"image/draw"
	"math"
)

// DecodingCfg contains the parameters used for image decoding. Decoding will
// use default values for uninitialized members.
type DecodingCfg struct {
	BaseSize        int     // the base image size (default: 32px)
	SaturationBoost float64 // the factor applied to increase image saturation (default: 1.25)
}

// EncodeImage returns the binary hash of an image.
func EncodeImage(img image.Image) []byte {
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
	hash := Hash{
		LDC:      lDC,
		PDC:      pDC,
		QDC:      qDC,
		LScale:   lScale,
		HasAlpha: hasAlpha,

		Lx:          lx,
		Ly:          ly,
		PScale:      pScale,
		QScale:      qScale,
		IsLandscape: isLandscape == 1,

		ADC:    aDC,
		AScale: aScale,

		LAC: lAC,
		PAC: pAC,
		QAC: qAC,

		AAC: aAC,
	}

	return hash.Encode()
}

// DecodeImage returns the image associated with a binary hash using the
// default decoding configuration.
func DecodeImage(hashData []byte) (image.Image, error) {
	return DecodeImageWithCfg(hashData, DecodingCfg{})
}

// DecodeImageWithCfg returns the image associated with a binary hash.
func DecodeImageWithCfg(hashData []byte, cfg DecodingCfg) (image.Image, error) {
	// Configuration default values
	if cfg.BaseSize == 0 {
		cfg.BaseSize = 32
	}

	if cfg.SaturationBoost == 0.0 {
		cfg.SaturationBoost = 1.25
	}

	// Read the content of the hash
	var hash Hash
	if err := hash.Decode(hashData, &cfg); err != nil {
		return nil, err
	}

	// Prepare the image
	w, h := hash.Size(cfg.BaseSize)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	data := img.Pix

	// Decode RGBA data
	idx := 0

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := hash.coefficients(x, y, w, h)

			// Decode L
			l := hash.LDC

			j := 0
			for cy := 0; cy < hash.Ly; cy++ {
				cx := 0
				if cy == 0 {
					cx = 1
				}

				fy2 := fy[cy] * 2.0
				for ; cx*hash.Ly < hash.Lx*(hash.Ly-cy); cx++ {
					l += hash.LAC[j] * fx[cx] * fy2
					j++
				}
			}

			// Decode P and Q
			p := hash.PDC
			q := hash.QDC

			j = 0
			for cy := 0; cy < 3; cy++ {
				cx := 0
				if cy == 0 {
					cx = 1
				}

				fy2 := fy[cy] * 2.0
				for ; cx < 3-cy; cx++ {
					f := fx[cx] * fy2
					p += hash.PAC[j] * f
					q += hash.QAC[j] * f
					j++
				}
			}

			// Decode A
			a := hash.ADC

			if hash.HasAlpha {
				j = 0
				for cy := 0; cy < 5; cy++ {
					cx := 0
					if cy == 0 {
						cx = 1
					}

					fy2 := fy[cy] * 2.0
					for ; cx < 5-cy; cx++ {
						a += hash.AAC[j] * fx[cx] * fy2
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
