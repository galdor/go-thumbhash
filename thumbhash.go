package thumbhash

import (
	"image"
	"image/color"
)

type Hash = []byte

func Encode(img image.Image) Hash {
	// TODO
	return nil
}

func Decode(Hash) (image.Image, error) {
	// TODO
	return nil, nil
}

func AverageColor(hash Hash) (color.Color, error) {
	// TODO
	return nil, nil
}

func ApproximateAspectRatio(hash Hash) (float64, error) {
	// TODO
	return 0.0, nil
}
