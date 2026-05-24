package posters

import (
	"bytes"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
)

const (
	visualSampleSize = 16
	visualHashBits   = visualSampleSize * visualSampleSize
	colorBuckets     = 4 * 4 * 4
)

type visualFingerprint struct {
	width      int
	height     int
	avgHash    [visualHashBits]bool
	diffHash   [(visualSampleSize - 1) * visualSampleSize]bool
	colorHist  [colorBuckets]float64
	lumaMean   float64
	lumaStdDev float64
}

func imageFingerprint(data []byte) (visualFingerprint, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return visualFingerprint{}, err
	}
	return fingerprintImage(img), nil
}

func imageFingerprints(data []byte) ([]visualFingerprint, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	fps := []visualFingerprint{fingerprintImageBounds(img, bounds)}
	for _, b := range []image.Rectangle{trimPosterBorderBounds(img), trimLightPosterBorderBounds(img), insetBounds(bounds, 0.02), insetBounds(bounds, 0.04)} {
		if b.Empty() || b == bounds || b.Dx() < visualSampleSize || b.Dy() < visualSampleSize {
			continue
		}
		fps = append(fps, fingerprintImageBounds(img, b))
	}
	return fps, nil
}

func fingerprintImage(img image.Image) visualFingerprint {
	return fingerprintImageBounds(img, img.Bounds())
}

func fingerprintImageBounds(img image.Image, bounds image.Rectangle) visualFingerprint {
	fp := visualFingerprint{width: bounds.Dx(), height: bounds.Dy()}
	luma := make([]float64, visualHashBits)
	sum := 0.0
	for y := 0; y < visualSampleSize; y++ {
		for x := 0; x < visualSampleSize; x++ {
			c := sampleImageBounds(img, bounds, x, y, visualSampleSize, visualSampleSize)
			r, g, b, _ := c.RGBA()
			rf, gf, bf := float64(r)/65535.0, float64(g)/65535.0, float64(b)/65535.0
			lum := 0.299*rf + 0.587*gf + 0.114*bf
			index := y*visualSampleSize + x
			luma[index] = lum
			sum += lum
			rb := min(3, int(rf*4))
			gb := min(3, int(gf*4))
			bb := min(3, int(bf*4))
			fp.colorHist[rb*16+gb*4+bb]++
		}
	}
	fp.lumaMean = sum / float64(len(luma))
	variance := 0.0
	for i, lum := range luma {
		fp.avgHash[i] = lum >= fp.lumaMean
		delta := lum - fp.lumaMean
		variance += delta * delta
	}
	fp.lumaStdDev = math.Sqrt(variance / float64(len(luma)))
	for y := 0; y < visualSampleSize; y++ {
		for x := 0; x < visualSampleSize-1; x++ {
			fp.diffHash[y*(visualSampleSize-1)+x] = luma[y*visualSampleSize+x] > luma[y*visualSampleSize+x+1]
		}
	}
	for i := range fp.colorHist {
		fp.colorHist[i] /= float64(len(luma))
	}
	return fp
}

func sampleImage(img image.Image, x, y, w, h int) color.Color {
	return sampleImageBounds(img, img.Bounds(), x, y, w, h)
}

func sampleImageBounds(img image.Image, bounds image.Rectangle, x, y, w, h int) color.Color {
	if bounds.Empty() {
		bounds = img.Bounds()
	}
	px := bounds.Min.X + min(bounds.Dx()-1, max(0, int((float64(x)+0.5)*float64(bounds.Dx())/float64(w))))
	py := bounds.Min.Y + min(bounds.Dy()-1, max(0, int((float64(y)+0.5)*float64(bounds.Dy())/float64(h))))
	return img.At(px, py)
}

func trimPosterBorderBounds(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	trimmed := bounds
	maxXTrim := bounds.Dx() / 5
	maxYTrim := bounds.Dy() / 5
	for trimmed.Min.X < trimmed.Max.X-visualSampleSize && trimmed.Min.X-bounds.Min.X < maxXTrim && edgeLooksLikeBorder(img, trimmed, "left") {
		trimmed.Min.X++
	}
	for trimmed.Max.X > trimmed.Min.X+visualSampleSize && bounds.Max.X-trimmed.Max.X < maxXTrim && edgeLooksLikeBorder(img, trimmed, "right") {
		trimmed.Max.X--
	}
	for trimmed.Min.Y < trimmed.Max.Y-visualSampleSize && trimmed.Min.Y-bounds.Min.Y < maxYTrim && edgeLooksLikeBorder(img, trimmed, "top") {
		trimmed.Min.Y++
	}
	for trimmed.Max.Y > trimmed.Min.Y+visualSampleSize && bounds.Max.Y-trimmed.Max.Y < maxYTrim && edgeLooksLikeBorder(img, trimmed, "bottom") {
		trimmed.Max.Y--
	}
	return trimmed
}

func edgeLooksLikeBorder(img image.Image, bounds image.Rectangle, edge string) bool {
	const samples = 48
	borderLike := 0
	for i := 0; i < samples; i++ {
		var x, y int
		switch edge {
		case "left":
			x = bounds.Min.X
			y = bounds.Min.Y + i*bounds.Dy()/samples
		case "right":
			x = bounds.Max.X - 1
			y = bounds.Min.Y + i*bounds.Dy()/samples
		case "top":
			x = bounds.Min.X + i*bounds.Dx()/samples
			y = bounds.Min.Y
		default:
			x = bounds.Min.X + i*bounds.Dx()/samples
			y = bounds.Max.Y - 1
		}
		if pixelLooksLikeBorder(img.At(x, y)) {
			borderLike++
		}
	}
	return float64(borderLike)/samples >= 0.82
}

func pixelLooksLikeBorder(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	rf, gf, bf := float64(r)/65535.0, float64(g)/65535.0, float64(b)/65535.0
	lum := 0.299*rf + 0.587*gf + 0.114*bf
	spread := math.Max(rf, math.Max(gf, bf)) - math.Min(rf, math.Min(gf, bf))
	return spread < 0.08 && (lum > 0.88 || lum < 0.08)
}

func trimLightPosterBorderBounds(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	maxXTrim := bounds.Dx() * 35 / 100
	maxYTrim := bounds.Dy() * 35 / 100
	left := findContentColumn(img, bounds, bounds.Min.X, bounds.Max.X, 1, maxXTrim)
	right := findContentColumn(img, bounds, bounds.Max.X-1, bounds.Min.X-1, -1, maxXTrim)
	top := findContentRow(img, bounds, bounds.Min.Y, bounds.Max.Y, 1, maxYTrim)
	bottom := findContentRow(img, bounds, bounds.Max.Y-1, bounds.Min.Y-1, -1, maxYTrim)
	trimmed := image.Rect(left, top, right+1, bottom+1)
	if trimmed.Dx() < visualSampleSize || trimmed.Dy() < visualSampleSize || trimmed == bounds {
		return bounds
	}
	return trimmed
}

func findContentColumn(img image.Image, bounds image.Rectangle, start, stop, step, maxTrim int) int {
	trimmed := 0
	for x := start; x != stop && trimmed < maxTrim; x += step {
		if columnHasPosterContent(img, bounds, x) {
			return x
		}
		trimmed++
	}
	return start
}

func findContentRow(img image.Image, bounds image.Rectangle, start, stop, step, maxTrim int) int {
	trimmed := 0
	for y := start; y != stop && trimmed < maxTrim; y += step {
		if rowHasPosterContent(img, bounds, y) {
			return y
		}
		trimmed++
	}
	return start
}

func columnHasPosterContent(img image.Image, bounds image.Rectangle, x int) bool {
	const samples = 64
	content := 0
	for i := 0; i < samples; i++ {
		y := bounds.Min.Y + i*bounds.Dy()/samples
		if !pixelLooksLikeLightBorder(img.At(x, y)) {
			content++
		}
	}
	return float64(content)/samples >= 0.18
}

func rowHasPosterContent(img image.Image, bounds image.Rectangle, y int) bool {
	const samples = 64
	content := 0
	for i := 0; i < samples; i++ {
		x := bounds.Min.X + i*bounds.Dx()/samples
		if !pixelLooksLikeLightBorder(img.At(x, y)) {
			content++
		}
	}
	return float64(content)/samples >= 0.18
}

func pixelLooksLikeLightBorder(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	rf, gf, bf := float64(r)/65535.0, float64(g)/65535.0, float64(b)/65535.0
	lum := 0.299*rf + 0.587*gf + 0.114*bf
	spread := math.Max(rf, math.Max(gf, bf)) - math.Min(rf, math.Min(gf, bf))
	return lum > 0.82 && spread < 0.16
}

func insetBounds(bounds image.Rectangle, frac float64) image.Rectangle {
	dx := int(float64(bounds.Dx()) * frac)
	dy := int(float64(bounds.Dy()) * frac)
	return image.Rect(bounds.Min.X+dx, bounds.Min.Y+dy, bounds.Max.X-dx, bounds.Max.Y-dy)
}

func maxVisualSimilarity(a, b []visualFingerprint) float64 {
	best := 0.0
	for _, left := range a {
		for _, right := range b {
			best = math.Max(best, visualSimilarity(left, right))
		}
	}
	return best
}

func visualSimilarity(a, b visualFingerprint) float64 {
	avg := boolSimilarity(a.avgHash[:], b.avgHash[:])
	diff := boolSimilarity(a.diffHash[:], b.diffHash[:])
	color := histogramIntersection(a.colorHist[:], b.colorHist[:])
	aspect := aspectSimilarity(a.width, a.height, b.width, b.height)
	contrast := 1 - math.Min(1, math.Abs(a.lumaStdDev-b.lumaStdDev))
	return 0.44*avg + 0.16*diff + 0.30*color + 0.06*aspect + 0.04*contrast
}

func boolSimilarity(a, b []bool) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	return float64(same) / float64(len(a))
}

func histogramIntersection(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	sum := 0.0
	for i := range a {
		sum += math.Min(a[i], b[i])
	}
	return sum
}

func aspectSimilarity(aw, ah, bw, bh int) float64 {
	if aw <= 0 || ah <= 0 || bw <= 0 || bh <= 0 {
		return 0
	}
	a := float64(aw) / float64(ah)
	b := float64(bw) / float64(bh)
	return math.Max(0, math.Min(1, math.Min(a, b)/math.Max(a, b)))
}
