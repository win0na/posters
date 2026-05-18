package posters

import (
	"bytes"
	"image"
	"image/color"
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

func fingerprintImage(img image.Image) visualFingerprint {
	bounds := img.Bounds()
	fp := visualFingerprint{width: bounds.Dx(), height: bounds.Dy()}
	luma := make([]float64, visualHashBits)
	sum := 0.0
	for y := 0; y < visualSampleSize; y++ {
		for x := 0; x < visualSampleSize; x++ {
			c := sampleImage(img, x, y, visualSampleSize, visualSampleSize)
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
	bounds := img.Bounds()
	px := bounds.Min.X + min(bounds.Dx()-1, max(0, int((float64(x)+0.5)*float64(bounds.Dx())/float64(w))))
	py := bounds.Min.Y + min(bounds.Dy()-1, max(0, int((float64(y)+0.5)*float64(bounds.Dy())/float64(h))))
	return img.At(px, py)
}

func visualSimilarity(a, b visualFingerprint) float64 {
	avg := boolSimilarity(a.avgHash[:], b.avgHash[:])
	diff := boolSimilarity(a.diffHash[:], b.diffHash[:])
	color := histogramIntersection(a.colorHist[:], b.colorHist[:])
	aspect := aspectSimilarity(a.width, a.height, b.width, b.height)
	contrast := 1 - math.Min(1, math.Abs(a.lumaStdDev-b.lumaStdDev))
	return 0.36*avg + 0.28*diff + 0.26*color + 0.06*aspect + 0.04*contrast
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
