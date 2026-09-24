package layout

import "image"

type Fixed int

func SplitVertical(r image.Rectangle, f Fixed) (top, bottom image.Rectangle) {
	height := int(f)
	if height > r.Dy() {
		height = r.Dy()
	}
	top = image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+height)
	bottom = image.Rect(r.Min.X, r.Min.Y+height, r.Max.X, r.Max.Y)
	return
}

func SplitHorizontal(r image.Rectangle, f Fixed) (left, right image.Rectangle) {
	width := int(f)
	if width > r.Dx() {
		width = r.Dx()
	}
	left = image.Rect(r.Min.X, r.Min.Y, r.Min.X+width, r.Max.Y)
	right = image.Rect(r.Min.X+width, r.Min.Y, r.Max.X, r.Max.Y)
	return
}
