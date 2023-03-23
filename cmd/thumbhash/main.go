package main

import (
	"encoding/base64"
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/exograd/go-program"
	"github.com/galdor/go-thumbhash"

	"image/draw"
	_ "image/jpeg"
	_ "image/png"
)

func main() {
	var c *program.Command

	p := program.NewProgram("thumbhash",
		"utilities for the go-thumbhash image placeholder generation library")

	c = p.AddCommand("image-to-raw-data", "convert an image to a raw data file",
		cmdImageToRawData)
	c.AddArgument("path", "the path of the image to decode")
	c.AddOption("o", "output", "path", "", "the path to write decoded data to")

	c = p.AddCommand("encode-image", "compute the hash of an image file",
		cmdEncodeImage)
	c.AddArgument("path", "the path of the image to encode")

	p.ParseCommandLine()
	p.Run()
}

func cmdImageToRawData(p *program.Program) {
	filePath := p.ArgumentValue("path")

	var outputPath string
	if output := p.OptionValue("output"); output != "" {
		outputPath = output
	} else {
		ext := filepath.Ext(filePath)
		outputPath = filePath[:len(filePath)-len(ext)] + ".data"
	}

	img, err := readImage(filePath)
	if err != nil {
		p.Fatal("cannot read image from %q: %v", filePath, err)
	}

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	data := rgba.Pix

	if err := ioutil.WriteFile(outputPath, data, 0644); err != nil {
		p.Fatal("cannot write %q: %v", outputPath, err)
	}
}

func cmdEncodeImage(p *program.Program) {
	filePath := p.ArgumentValue("path")

	img, err := readImage(filePath)
	if err != nil {
		p.Fatal("cannot read image from %q: %v", filePath, err)
	}

	hash := thumbhash.Encode(img)

	fmt.Println(base64.StdEncoding.EncodeToString(hash))
}

func readImage(filePath string) (image.Image, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("cannot decode file: %w", err)
	}

	return img, nil
}
