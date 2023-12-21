package thumbhash

import (
	"encoding/base64"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"testing"
)

func TestEncodeImage(t *testing.T) {
	checkImage := func(expectedB64Hash, filePath string) {
		file, err := os.Open(filePath)
		if err != nil {
			t.Fatalf("cannot open %q: %v", filePath, err)
		}
		defer file.Close()

		img, _, err := image.Decode(file)
		if err != nil {
			t.Fatalf("cannot decode %q: %v", filePath, err)
		}

		hash := EncodeImage(img)
		b64Hash := base64.StdEncoding.EncodeToString(hash)

		if b64Hash != expectedB64Hash {
			t.Errorf("hash of %q is %q but should be %q",
				filePath, b64Hash, expectedB64Hash)
		}
	}

	checkImage("1QcSHQRnh493V4dIh4eXh1h4kJUI", "data/sunrise.jpg")
	checkImage("X5qGNQw7oElslqhGWfSE+Q6oJ1h2iHB2Rw==", "data/firefox.png")
	checkImage("VvYRNQRod313B4h3eHhYiHeAiQUo", "data/large-sunrise.png")
}
