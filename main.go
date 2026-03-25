package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/EdlinOrg/prominentcolor"
	_ "github.com/gen2brain/heic"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/nfnt/resize"
)

type ANSIColor struct {
	R, G, B int
}

func (c ANSIColor) Hex() string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func adjustColor(c colorful.Color, minL, maxL, minS, maxS *float64) colorful.Color {
	h, s, l := c.Hsl()

	if minL != nil && l < *minL {
		l = *minL
	}
	if maxL != nil && l > *maxL {
		l = *maxL
	}
	if minS != nil && s < *minS {
		s = *minS
	}
	if maxS != nil && s > *maxS {
		s = *maxS
	}

	return colorful.Hsl(h, s, l)
}

func getClosestColor(targetHue float64, colors []colorful.Color) colorful.Color {
	bestColor := colors[0]
	minDist := math.MaxFloat64

	for _, c := range colors {
		h, s, _ := c.Hsl()
		// Hue distance (circular)
		dist := math.Min(math.Abs(h-targetHue), 360-math.Abs(h-targetHue)) / 360.0

		// Prioritize saturation slightly
		weightedDist := dist * (2.0 - s)

		if weightedDist < minDist {
			minDist = weightedDist
			bestColor = c
		}
	}
	return bestColor
}

func main() {
	var (
		imgPath    = flag.String("i", "", "Input image path")
		outputName = flag.String("o", "", "Output name (for .itermcolors)")
		apply      = flag.Bool("a", true, "Apply colors to current terminal")
	)
	flag.Parse()

	if *imgPath == "" {
		fmt.Println("Usage: img2iterm -i <image_path> [-o <output_name>]")
		os.Exit(1)
	}

	file, err := os.Open(*imgPath)
	if err != nil {
		fmt.Printf("Error opening image: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Printf("Error decoding image: %v\n", err)
		os.Exit(1)
	}

	// Resize for speed
	img = resize.Resize(100, 100, img, resize.Lanczos3)

	// Extract dominant colors
	res, err := prominentcolor.Kmeans(img)
	if err != nil {
		fmt.Printf("Error extracting colors: %v\n", err)
		os.Exit(1)
	}

	var extracted []colorful.Color
	for _, k := range res {
		c := colorful.Color{
			R: float64(k.Color.R) / 255.0,
			G: float64(k.Color.G) / 255.0,
			B: float64(k.Color.B) / 255.0,
		}
		extracted = append(extracted, c)
	}

	if len(extracted) == 0 {
		fmt.Println("No colors extracted.")
		os.Exit(1)
	}

	// Logic from Python script
	sort.Slice(extracted, func(i, j int) bool {
		_, _, li := extracted[i].Hsl()
		_, _, lj := extracted[j].Hsl()
		return li < lj
	})

	l010 := 0.10
	l090 := 0.90
	l030 := 0.30
	l070 := 0.70
	s050 := 0.50
	s060 := 0.60
	l050 := 0.50

	bg := adjustColor(extracted[0], nil, &l010, nil, nil)
	fg := adjustColor(extracted[len(extracted)-1], &l090, nil, nil, nil)

	ansi := make(map[int]colorful.Color)
	ansi[0] = bg                                             // Black
	ansi[7] = adjustColor(fg, nil, &l070, nil, nil)          // White
	ansi[8] = adjustColor(bg, &l030, nil, nil, nil)          // Bright Black
	ansi[15] = fg                                            // Bright White

	targets := map[int]float64{
		1: 0.0,   // Red
		2: 120.0, // Green
		3: 60.0,  // Yellow
		4: 240.0, // Blue
		5: 300.0, // Magenta
		6: 180.0, // Cyan
	}

	for i, hue := range targets {
		base := getClosestColor(hue, extracted)
		// Normal
		ansi[i] = adjustColor(base, &l050, &l070, &s050, nil)
		// Bright
		ansi[i+8] = adjustColor(base, &l070, &l090, &s060, nil)
	}

	if *apply {
		applyColors(bg, fg, ansi)
	}

	if *outputName != "" || !*apply {
		name := *outputName
		if name == "" {
			name = filepath.Base(*imgPath)
			name = name[:len(name)-len(filepath.Ext(name))]
		}
		
		outputFile := name + ".itermcolors"
		err := saveItermColors(outputFile, bg, fg, ansi)
		if err != nil {
			fmt.Printf("Error saving .itermcolors: %v\n", err)
		} else {
			fmt.Printf("Saved to %s\n", outputFile)
		}
	}
}

func saveItermColors(path string, bg, fg colorful.Color, ansi map[int]colorful.Color) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(f, `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`)
	fmt.Fprintln(f, `<plist version="1.0">`)
	fmt.Fprintln(f, `<dict>`)

	writeColor := func(name string, c colorful.Color) {
		fmt.Fprintf(f, "    <key>%s</key>\n", name)
		fmt.Fprintln(f, "    <dict>")
		fmt.Fprintf(f, "        <key>Blue Component</key>\n        <real>%f</real>\n", c.B)
		fmt.Fprintf(f, "        <key>Green Component</key>\n        <real>%f</real>\n", c.G)
		fmt.Fprintf(f, "        <key>Red Component</key>\n        <real>%f</real>\n", c.R)
		fmt.Fprintln(f, "        <key>Color Space</key>\n        <string>sRGB</string>")
		fmt.Fprintln(f, "    </dict>")
	}

	writeColor("Background Color", bg)
	writeColor("Foreground Color", fg)
	writeColor("Cursor Color", fg)
	writeColor("Selected Text Color", bg)
	writeColor("Selection Color", fg)

	for i := 0; i < 16; i++ {
		writeColor(fmt.Sprintf("Ansi %d Color", i), ansi[i])
	}

	fmt.Fprintln(f, `</dict>`)
	fmt.Fprintln(f, `</plist>`)
	return nil
}

func applyColors(bg, fg colorful.Color, ansi map[int]colorful.Color) {
	// Standard ANSI escape sequences to set terminal colors
	// \033]4;{index};{color}\a
	for i := 0; i < 16; i++ {
		c := ansi[i]
		fmt.Printf("\033]4;%d;%s\033\\", i, c.Hex())
	}

	// Background, Foreground, Cursor
	fmt.Printf("\033]10;%s\033\\", fg.Hex())
	fmt.Printf("\033]11;%s\033\\", bg.Hex())
	fmt.Printf("\033]12;%s\033\\", fg.Hex())
}
