//go:build cgo && gocv

package ml

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"

	"gocv.io/x/gocv"
)

const faceCascadeEnv = "SALAMEY_FACE_CASCADE"

type FaceDetector struct {
	cascade *gocv.CascadeClassifier
}

func NewFaceDetector(useCgo bool) (*FaceDetector, error) {
	if !useCgo {
		return nil, errors.New("face detection requires CGO and the gocv build tag")
	}

	path := strings.TrimSpace(os.Getenv(faceCascadeEnv))
	if path == "" {
		return nil, fmt.Errorf("set %s to a Haar cascade XML file", faceCascadeEnv)
	}

	cascade := gocv.NewCascadeClassifier()
	if !cascade.Load(path) {
		_ = cascade.Close()
		return nil, fmt.Errorf("failed to load face cascade %q", path)
	}

	return &FaceDetector{cascade: &cascade}, nil
}

func (fd *FaceDetector) DetectFaces(img image.Image) ([]image.Rectangle, error) {
	if fd == nil || fd.cascade == nil {
		return nil, errors.New("face detector is not initialized")
	}
	if img == nil {
		return nil, errors.New("face detection received a nil image")
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return nil, fmt.Errorf("failed to encode image: %w", err)
	}

	mat, err := gocv.IMDecode(encoded.Bytes(), gocv.IMReadColor)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}
	defer mat.Close()

	return fd.cascade.DetectMultiScale(mat), nil
}

func (fd *FaceDetector) Close() {
	if fd != nil && fd.cascade != nil {
		_ = fd.cascade.Close()
		fd.cascade = nil
	}
}
