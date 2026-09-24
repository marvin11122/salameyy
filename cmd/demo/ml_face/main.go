//go:build cgo && gocv

package main

import (
	"fmt"
	"image/color"
	"log"
	"os"

	"github.com/wiesnermarven-hue/salameyy/internal/ml"
	"gocv.io/x/gocv"
)

func main() {
	fd, err := ml.NewFaceDetector(len(os.Getenv("DISABLE_CGO")) == 0)
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()

	webcam, err := gocv.OpenVideoCapture(0)
	if err != nil {
		log.Fatal(err)
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	window := gocv.NewWindow("Face Detection")
	defer window.Close()

	for {
		if ok := webcam.Read(&img); !ok {
			log.Println("Cannot read from webcam")
			break
		}
		if img.Empty() {
			continue
		}

		imgGo, err := img.ToImage()
		if err != nil {
			log.Println(err)
			continue
		}
		faces, err := fd.DetectFaces(imgGo)
		if err != nil {
			log.Println(err)
			continue
		}

		for _, face := range faces {
			gocv.Rectangle(&img, face, color.RGBA{R: 255, A: 0}, 2)
		}

		fmt.Print("\033[H\033[2J")
		fmt.Println(ml.RenderANSI(imgGo, 80))
		window.IMShow(img)
		if window.WaitKey(1) >= 0 {
			break
		}
	}
}
