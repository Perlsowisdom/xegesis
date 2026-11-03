package main

import (
	"context"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// screenshotDelay is how long the window draws before its picture is taken: about 120 frames at
// 60 frames a second, plus time for the interface to load its first section.
const screenshotDelay = 4 * time.Second

// screenshotArg returns the file given with --screenshot <file.png> (or --screenshot=<file.png>).
func screenshotArg(args []string) string {
	for i, a := range args {
		if a == "--screenshot" && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "--screenshot="); ok {
			return v
		}
	}
	return ""
}

// takeScreenshot waits for the window to draw, saves its picture as a PNG and quits the app.
func takeScreenshot(ctx context.Context, path string) {
	go func() {
		// XEGESIS_OPEN names a section to show first, such as "sermons", to check one screen.
		if section := os.Getenv("XEGESIS_OPEN"); section != "" {
			time.Sleep(screenshotDelay / 2)
			runtime.WindowExecJS(ctx, "location.hash = "+strconv.Quote("#"+section)+";")
		}
		time.Sleep(screenshotDelay)
		if err := saveWindowPNG(path); err != nil {
			log.Printf("screenshot: %v", err)
		}
		runtime.Quit(ctx)
	}()
}

func saveWindowPNG(path string) error {
	img, err := captureWindow()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
