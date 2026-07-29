package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/chromedp/chromedp"
)

// captureThumbnail erstellt einen Screenshot der Seite und speichert ihn als
// JPEG in ThumbDir. Gibt den Dateinamen zurück, oder "" bei Fehler.
func (a *App) captureThumbnail(pageURL string) string {
	sum := sha1.Sum([]byte(pageURL + ":" + strconv.FormatInt(time.Now().UnixNano(), 10)))
	name := hex.EncodeToString(sum[:8]) + ".jpg"
	dst := filepath.Join(a.cfg.ThumbDir, name)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("headless", true),
	)
	if a.cfg.ChromePath != "" {
		opts = append(opts, chromedp.ExecPath(a.cfg.ChromePath))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	var pngBuf []byte
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1280, 800),
		chromedp.Navigate(pageURL),
		chromedp.Sleep(1500*time.Millisecond),
		chromedp.CaptureScreenshot(&pngBuf),
	)
	if err != nil || len(pngBuf) == 0 {
		return ""
	}

	jpgBuf, err := pngToJPEG(pngBuf)
	if err != nil {
		return ""
	}
	if err := os.WriteFile(dst, jpgBuf, 0o644); err != nil {
		return ""
	}
	return name
}

func pngToJPEG(pngData []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 72}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (a *App) deleteThumbnail(name string) {
	if name == "" {
		return
	}
	_ = os.Remove(filepath.Join(a.cfg.ThumbDir, filepath.Base(name)))
}
