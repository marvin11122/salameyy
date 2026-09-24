package ml

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/disintegration/imaging"
)

var ANSIBlockChars = []string{" ", "░", "▒", "▓", "█"}

func RenderANSI(img image.Image, width int) string {
	if img == nil || width <= 0 {
		return ""
	}

	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return ""
	}

	height := max(1, width*bounds.Dy()/bounds.Dx())
	resized := imaging.Resize(img, width, height, imaging.Lanczos)
	var output strings.Builder
	for y := bounds.Min.Y; y < bounds.Min.Y+height; y++ {
		for x := bounds.Min.X; x < bounds.Min.X+width; x++ {
			pixel := color.NRGBAModel.Convert(resized.At(x-bounds.Min.X, y-bounds.Min.Y)).(color.NRGBA)
			gray := 0.299*float64(pixel.R) + 0.587*float64(pixel.G) + 0.114*float64(pixel.B)
			index := min(len(ANSIBlockChars)-1, int(gray/64))
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", pixel.R, pixel.G, pixel.B)))
			output.WriteString(style.Render(ANSIBlockChars[index]))
		}
		if y < bounds.Min.Y+height-1 {
			output.WriteByte('\n')
		}
	}
	return output.String()
}

func RenderSixel(image.Image) (string, error) {
	return "", fmt.Errorf("sixel rendering is not supported")
}
