//go:build !windows

package main

import (
	"errors"
	"image"
)

func captureWindow() (image.Image, error) {
	return nil, errors.New("--screenshot is only available on Windows for now")
}
