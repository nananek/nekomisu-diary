// One-shot tool: strip EXIF from all JPEG files under an uploads directory
// by decoding and re-encoding in place. Safe to run multiple times (idempotent
// after one successful pass — re-encodes still strip but are no-ops for EXIF).
package main

import (
	"flag"
	"fmt"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	uploadsDir := flag.String("uploads", "./uploads", "uploads directory")
	dryRun := flag.Bool("dry-run", false, "report only, don't rewrite")
	quality := flag.Int("quality", 95, "JPEG quality for re-encoding")
	flag.Parse()

	var total, rewritten, failed int
	err := filepath.Walk(*uploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" {
			return nil
		}
		total++

		origSize := info.Size()

		// Decode
		f, err := os.Open(path)
		if err != nil {
			log.Printf("  open %s: %v", path, err)
			failed++
			return nil
		}
		img, err := jpeg.Decode(f)
		f.Close()
		if err != nil {
			log.Printf("  decode %s: %v", path, err)
			failed++
			return nil
		}

		if *dryRun {
			fmt.Printf("would rewrite: %s (%d bytes)\n", path, origSize)
			rewritten++
			return nil
		}

		// Write to .tmp then rename
		tmp := path + ".tmp"
		out, err := os.Create(tmp)
		if err != nil {
			log.Printf("  create %s: %v", tmp, err)
			failed++
			return nil
		}
		if err := jpeg.Encode(out, img, &jpeg.Options{Quality: *quality}); err != nil {
			out.Close()
			os.Remove(tmp)
			log.Printf("  encode %s: %v", path, err)
			failed++
			return nil
		}
		out.Close()
		if err := os.Rename(tmp, path); err != nil {
			log.Printf("  rename %s: %v", path, err)
			failed++
			return nil
		}
		if newInfo, err := os.Stat(path); err == nil {
			fmt.Printf("%s  %d → %d\n", path, origSize, newInfo.Size())
		}
		rewritten++
		return nil
	})
	if err != nil {
		log.Fatalf("walk: %v", err)
	}
	log.Printf("JPEGs scanned: %d, rewritten: %d, failed: %d (dry-run=%v)",
		total, rewritten, failed, *dryRun)
}
