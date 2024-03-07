package thumbhash

import (
	"errors"
	"math"
)

var (
	ErrInvalidHash = errors.New("invalid hash")
)

// Hash binary representation:
//
// L DC:        6 bit
// P DC:        6 bit
// Q DC:        6 bit
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

// Hash represents the set of data stored in an image hash.
type Hash struct {
	LDC      float64
	PDC      float64
	QDC      float64
	LScale   float64
	HasAlpha bool

	Lx          int
	Ly          int
	PScale      float64
	QScale      float64
	IsLandscape bool

	ADC    float64 // if HasAlpha
	AScale float64 // if HasAlpha

	LAC []float64
	PAC []float64
	QAC []float64
	AAC []float64 // if HasAlpha
}

// Encode returns the binary representation of a hash.
func (h *Hash) Encode() []byte {
	// Compute the size of the hash
	nbAC := len(h.LAC) + len(h.PAC) + len(h.QAC)
	if h.HasAlpha {
		nbAC += len(h.AAC)
	}
	hashSize := 3 + 2 + (nbAC+1)/2
	if h.HasAlpha {
		hashSize += 1
	}

	hash := make([]byte, hashSize)

	// First block (3 bytes)
	header24 := iround(63.0 * h.LDC)
	header24 |= iround(31.5+31.5*h.PDC) << 6
	header24 |= iround(31.5+31.5*h.QDC) << 12
	header24 |= iround(31.0*h.LScale) << 18
	if h.HasAlpha {
		header24 |= 1 << 23
	}

	hash[0] = byte(header24)
	hash[1] = byte(header24 >> 8)
	hash[2] = byte(header24 >> 16)

	// Second block (2 bytes)
	lCount := h.Lx
	if h.IsLandscape {
		lCount = h.Ly
	}

	header16 := lCount
	header16 |= iround(63.0*h.PScale) << 3
	header16 |= iround(63.0*h.QScale) << 9
	if h.IsLandscape {
		header16 |= 1 << 15
	}

	hash[3] = byte(header16)
	hash[4] = byte(header16 >> 8)

	// Alpha data
	if h.HasAlpha {
		hash[5] = byte(iround(15.0*h.ADC) | iround(15.0*h.AScale)<<4)
	}

	// AC coefficients
	acs := [][]float64{h.LAC, h.PAC, h.QAC}
	start := 5
	if h.HasAlpha {
		acs = append(acs, h.AAC)
		start = 6
	}

	idx := 0

	for _, ac := range acs {
		for _, f := range ac {
			// hash[start+(idx/2)] |= byte(iround(15.0*f) << ((idx & 1) * 4))
			hash[start+(idx>>1)] |= byte(iround(15*f) << ((idx & 1) << 2))
			idx += 1
		}
	}

	return hash
}

// Decode extract data from the binary representation of a hash.
func (h *Hash) Decode(data []byte, cfg *DecodingCfg) error {
	if len(data) < 5 {
		return ErrInvalidHash
	}

	// First block
	header24 := int(data[0]) | int(data[1])<<8 | int(data[2])<<16

	h.LDC = float64(header24&63) / 63.0
	h.PDC = float64((header24>>6)&63)/31.5 - 1.0
	h.QDC = float64((header24>>12)&63)/31.5 - 1.0
	h.LScale = float64((header24>>18)&31) / 31.0
	h.HasAlpha = (header24 >> 23) != 0

	// Second block
	header16 := int(data[3]) | int(data[4])<<8

	h.PScale = float64((header16>>3)&63) / 63.0
	h.QScale = float64((header16>>9)&63) / 63.0
	h.IsLandscape = (header16 >> 15) != 0

	lCount := int(header16 & 7)
	if h.IsLandscape {
		if h.HasAlpha {
			h.Lx = 5
		} else {
			h.Lx = 7
		}
		h.Ly = max(3, lCount)
	} else {
		h.Lx = max(3, lCount)
		if h.HasAlpha {
			h.Ly = 5
		} else {
			h.Ly = 7
		}
	}

	// Alpha data
	h.ADC = 1.0
	h.AScale = 0.0

	if h.HasAlpha {
		if len(data) < 6 {
			return ErrInvalidHash
		}

		h.ADC = float64(data[5]&15) / 15.0
		h.AScale = float64(data[5]>>4) / 15.0
	}

	// DC coefficients
	start := 5
	if h.HasAlpha {
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
				if hidx >= len(data) {
					err = ErrInvalidHash
					return nil
				}

				f := (float64((data[hidx]>>((idx&1)*4))&15)/7.5 - 1.0) * scale
				ac = append(ac, f)
				idx++
			}
		}

		return
	}

	// Note the multiplication by a constant factor to increase saturation
	// since quantization tend to produce dull images.
	h.LAC = decodeChannel(h.Lx, h.Ly, h.LScale)
	h.PAC = decodeChannel(3, 3, h.PScale*cfg.SaturationBoost)
	h.QAC = decodeChannel(3, 3, h.QScale*cfg.SaturationBoost)

	if h.HasAlpha {
		h.AAC = decodeChannel(5, 5, h.AScale)
	}

	return err
}

// Size return the width and height of the image associated with a hash
// according to a specific base size.
func (hash *Hash) Size(baseSize int) (w int, h int) {
	ratio := float64(hash.Lx) / float64(hash.Ly)

	if ratio > 1.0 {
		w = baseSize
		h = iround(float64(baseSize) / ratio)
	} else {
		w = iround(float64(baseSize) * ratio)
		h = baseSize
	}

	return
}

func (hash *Hash) coefficients(x, y, w, h int) (fx []float64, fy []float64) {
	xf := float64(x)
	yf := float64(y)

	wf, hf := float64(w), float64(h)

	n := 3
	if hash.HasAlpha {
		n = 5
	}
	n = max(hash.Lx, n)

	fx = make([]float64, n)
	for cx := 0; cx < n; cx++ {
		fx[cx] = math.Cos(math.Pi / wf * (xf + 0.5) * float64(cx))
	}

	n = 3
	if hash.HasAlpha {
		n = 5
	}
	n = max(hash.Ly, n)

	fy = make([]float64, n)
	for cy := 0; cy < n; cy++ {
		fy[cy] = math.Cos(math.Pi / hf * (yf + 0.5) * float64(cy))
	}

	return
}
