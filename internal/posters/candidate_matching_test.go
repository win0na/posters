package posters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/win0na/posters/internal/plex"
)

func TestChooseCandidateWithoutWikipediaUsesCanonical(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "John Wick", Year: 2014}
	candidates := []impCandidate{
		{Title: "John Wick", Year: 2014, PageURL: "http://www.impawards.com/2014/john_wick.html", ImageURL: "http://www.impawards.com/2014/posters/john_wick.jpg", Canonical: true},
		{Title: "John Wick", Year: 2014, PageURL: "http://www.impawards.com/2014/john_wick_ver2.html", ImageURL: "http://www.impawards.com/2014/posters/john_wick_ver2.jpg", Version: 2},
	}
	chosen, reason, err := chooseCandidate(movie, candidates, wikiPoster{})
	if err != nil {
		t.Fatalf("chooseCandidate() err = %v", err)
	}
	if chosen.PageURL != "http://www.impawards.com/2014/john_wick.html" || !strings.Contains(reason, "Wikipedia poster unavailable") {
		t.Fatalf("chosen=%#v reason=%q", chosen, reason)
	}
}

func TestIMPSearchURL(t *testing.T) {
	t.Parallel()

	got := impSearchURL("The Wild Robot 2024", 2)
	want := "http://www.impawards.com/cgi-bin/htsearch?words=The+Wild+Robot+2024;page=2"
	if got != want {
		t.Fatalf("impSearchURL() = %q, want %q", got, want)
	}
}

func TestChooseCandidateAmbiguous(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Alien", Year: 1979}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_ver2.html", ImageURL: "http://www.impawards.com/1979/posters/alien_ver2.jpg", Version: 2},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_ver3.html", ImageURL: "http://www.impawards.com/1979/posters/alien_ver3.jpg", Version: 3},
	}
	_, _, err := chooseCandidate(movie, candidates, wikiPoster{Poster: true})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("chooseCandidate() err = %v, want ambiguous", err)
	}
	var ambiguous *AmbiguousMatchError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("chooseCandidate() err = %T, want AmbiguousMatchError", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguous.Candidates = %#v", ambiguous.Candidates)
	}
	if ambiguous.Summary() != "2 candidates" {
		t.Fatalf("ambiguous.Summary() = %q", ambiguous.Summary())
	}
}

func TestChooseCandidateCanonical(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Alien", Year: 1979}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien.html", ImageURL: "http://www.impawards.com/1979/posters/alien.jpg", Canonical: true},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_ver2.html", ImageURL: "http://www.impawards.com/1979/posters/alien_ver2.jpg", Version: 2},
	}
	chosen, reason, err := chooseCandidate(movie, candidates, wikiPoster{Poster: true})
	if err != nil {
		t.Fatalf("chooseCandidate() err = %v", err)
	}
	if chosen.PageURL != "http://www.impawards.com/1979/alien.html" {
		t.Fatalf("chosen.PageURL = %q", chosen.PageURL)
	}
	if !strings.Contains(reason, "canonical") {
		t.Fatalf("reason = %q, want canonical", reason)
	}
}

func TestChooseCandidateWikipediaSignal(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Alien", Year: 1979}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_teaser.html", ImageURL: "http://www.impawards.com/1979/posters/alien_teaser.jpg"},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_regular_domestic.html", ImageURL: "http://www.impawards.com/1979/posters/alien_regular_domestic.jpg"},
	}
	wiki := wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/alien_regular_domestic_poster.jpg", Caption: "Theatrical regular domestic release poster"}
	chosen, reason, err := chooseCandidate(movie, candidates, wiki)
	if err != nil {
		t.Fatalf("chooseCandidate() err = %v", err)
	}
	if chosen.ImageURL != "http://www.impawards.com/1979/posters/alien_regular_domestic.jpg" {
		t.Fatalf("chosen.ImageURL = %q", chosen.ImageURL)
	}
	if !strings.Contains(reason, "token match") {
		t.Fatalf("reason = %q, want token match", reason)
	}
}

func TestChooseCandidateLowConfidenceStaysAmbiguous(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Alien", Year: 1979}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_red.html", ImageURL: "http://www.impawards.com/1979/posters/alien_red.jpg"},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_blue.html", ImageURL: "http://www.impawards.com/1979/posters/alien_blue.jpg"},
	}
	wiki := wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/alien_poster.jpg", Caption: "Theatrical release poster"}
	_, _, err := chooseCandidate(movie, candidates, wiki)
	var ambiguous *AmbiguousMatchError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("chooseCandidate() err = %T, want AmbiguousMatchError", err)
	}
}

func TestChooseVisualCandidatePicksImageMatch(t *testing.T) {
	blue := testPosterPNG(color.RGBA{R: 20, G: 40, B: 220, A: 255}, color.RGBA{R: 240, G: 240, B: 20, A: 255})
	red := testPosterPNG(color.RGBA{R: 220, G: 30, B: 20, A: 255}, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/wiki.png", "/1979/posters/alien_blue.png":
			_, _ = w.Write(blue)
		case "/1979/posters/alien_red.png":
			_, _ = w.Write(red)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client := server.Client()
	client.Transport = rewriteHostTransport{target: serverURL, base: http.DefaultTransport}
	service := &Service{http: client, cacheDir: t.TempDir()}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_red.html", ImageURL: "http://www.impawards.com/1979/posters/alien_red.png"},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_blue.html", ImageURL: "http://www.impawards.com/1979/posters/alien_blue.png"},
	}
	chosen, data, reason, err := service.chooseVisualCandidate(context.Background(), plex.Movie{Title: "Alien", Year: 1979}, candidates, wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/wiki.png"})
	if err != nil {
		t.Fatalf("chooseVisualCandidate() err = %v", err)
	}
	if chosen.ImageURL != "http://www.impawards.com/1979/posters/alien_blue.png" {
		t.Fatalf("chosen.ImageURL = %q", chosen.ImageURL)
	}
	if !bytes.Equal(data, blue) {
		t.Fatal("returned bytes are not chosen IMP image")
	}
	if !strings.Contains(reason, "visual match ") || !strings.Contains(reason, "%") {
		t.Fatalf("reason = %q", reason)
	}
}

func TestChooseVisualCandidateRejectsLowConfidenceMatch(t *testing.T) {
	blue := testPosterPNG(color.RGBA{R: 20, G: 40, B: 220, A: 255}, color.RGBA{R: 240, G: 240, B: 20, A: 255})
	red := testPosterPNG(color.RGBA{R: 220, G: 30, B: 20, A: 255}, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	green := testPosterPNG(color.RGBA{R: 20, G: 190, B: 50, A: 255}, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/wiki.png":
			_, _ = w.Write(blue)
		case "/1979/posters/alien_red.png":
			_, _ = w.Write(red)
		case "/1979/posters/alien_green.png":
			_, _ = w.Write(green)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client := server.Client()
	client.Transport = rewriteHostTransport{target: serverURL, base: http.DefaultTransport}
	service := &Service{http: client, cacheDir: t.TempDir()}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_red.html", ImageURL: "http://www.impawards.com/1979/posters/alien_red.png"},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_green.html", ImageURL: "http://www.impawards.com/1979/posters/alien_green.png"},
	}
	_, _, _, err = service.chooseVisualCandidate(context.Background(), plex.Movie{Title: "Alien", Year: 1979}, candidates, wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/wiki.png"})
	var ambiguous *AmbiguousMatchError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("chooseVisualCandidate() err = %T %v, want AmbiguousMatchError", err, err)
	}
	if len(ambiguous.Candidates) != 2 || !ambiguous.Candidates[0].HasVisualScore {
		t.Fatalf("ambiguous.Candidates = %#v, want visual scores", ambiguous.Candidates)
	}
	if !strings.Contains(ambiguous.Summary(), "best visual match") || !strings.Contains(ambiguous.Summary(), "%") {
		t.Fatalf("ambiguous.Summary() = %q, want visual percentage", ambiguous.Summary())
	}
}

func TestChooseVisualCandidateAllowsCloseHighConfidenceMatch(t *testing.T) {
	base := testPosterPNG(color.RGBA{R: 20, G: 40, B: 220, A: 255}, color.RGBA{R: 240, G: 240, B: 20, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/wiki.png", "/1986/posters/aliens_main.png", "/1986/posters/aliens_ver1.png":
			_, _ = w.Write(base)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client := server.Client()
	client.Transport = rewriteHostTransport{target: serverURL, base: http.DefaultTransport}
	service := &Service{http: client, cacheDir: t.TempDir()}
	candidates := []impCandidate{
		{Title: "Aliens", Year: 1986, PageURL: "http://www.impawards.com/1986/aliens.html", ImageURL: "http://www.impawards.com/1986/posters/aliens_main.png"},
		{Title: "Aliens", Year: 1986, PageURL: "http://www.impawards.com/1986/aliens_ver1.html", ImageURL: "http://www.impawards.com/1986/posters/aliens_ver1.png", Version: 1},
	}
	chosen, _, reason, err := service.chooseVisualCandidate(context.Background(), plex.Movie{Title: "Aliens", Year: 1986}, candidates, wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/wiki.png"})
	if err != nil {
		t.Fatalf("chooseVisualCandidate() err = %v", err)
	}
	if chosen.PageURL != "http://www.impawards.com/1986/aliens_ver1.html" || !strings.Contains(reason, "%") {
		t.Fatalf("chosen=%#v reason=%q", chosen, reason)
	}
}

func TestConfidentVisualMatchAllowsClearWinnerBelowHighThreshold(t *testing.T) {
	best := matchedCandidate{Score: 0.7298}
	second := matchedCandidate{Score: 0.4949}
	if !isConfidentVisualMatch(best, &second) {
		t.Fatal("isConfidentVisualMatch() rejected clear Aliens-style winner")
	}
}

func TestConfidentVisualMatchAllowsModerateClearWinnerBelowHighThreshold(t *testing.T) {
	best := matchedCandidate{Score: 0.7601}
	second := matchedCandidate{Score: 0.6590}
	if !isConfidentVisualMatch(best, &second) {
		t.Fatal("isConfidentVisualMatch() rejected Final-Destination-style winner")
	}
}

func TestConfidentVisualMatchAllowsSameImageNameHint(t *testing.T) {
	best := matchedCandidate{Score: 0.7671, NameHint: true}
	second := matchedCandidate{Score: 0.6968}
	if !isConfidentVisualMatch(best, &second) {
		t.Fatal("isConfidentVisualMatch() rejected same-image-name candidate")
	}
}

func TestConfidentVisualMatchRejectsWeakWinnerBelowHighThreshold(t *testing.T) {
	best := matchedCandidate{Score: 0.7298}
	second := matchedCandidate{Score: 0.66}
	if isConfidentVisualMatch(best, &second) {
		t.Fatal("isConfidentVisualMatch() accepted weak margin below high threshold")
	}
}

func TestSamePosterImageNameIgnoresWikiThumbAndIMPSizeSuffix(t *testing.T) {
	if !samePosterImageName("https://upload.wikimedia.org/wikipedia/en/thumb/7/7b/Exorcist_ver2.jpg/250px-Exorcist_ver2.jpg", "http://www.impawards.com/1973/posters/exorcist_ver2_xxlg.jpg") {
		t.Fatal("samePosterImageName() did not match wiki thumb with IMP size suffix")
	}
}

func TestVisualScorePercent(t *testing.T) {
	if got := visualMatchReason(0.9876); got != "visual match 98.8%" {
		t.Fatalf("visualMatchReason() = %q", got)
	}
	if got := fmt.Sprintf("%s; next best %s", visualMatchReason(0.9876), visualScorePercent(0.731)); got != "visual match 98.8%; next best 73.1%" {
		t.Fatalf("next best reason = %q", got)
	}
}

type rewriteHostTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func TestVisualSimilarityRanksCloserPosterHigher(t *testing.T) {
	base := testPosterPNG(color.RGBA{R: 10, G: 20, B: 200, A: 255}, color.RGBA{R: 240, G: 220, B: 20, A: 255})
	near := testPosterPNG(color.RGBA{R: 12, G: 22, B: 205, A: 255}, color.RGBA{R: 235, G: 215, B: 25, A: 255})
	far := testPosterPNG(color.RGBA{R: 200, G: 20, B: 20, A: 255}, color.RGBA{R: 20, G: 220, B: 20, A: 255})
	baseFP, err := imageFingerprint(base)
	if err != nil {
		t.Fatalf("imageFingerprint(base) err = %v", err)
	}
	nearFP, err := imageFingerprint(near)
	if err != nil {
		t.Fatalf("imageFingerprint(near) err = %v", err)
	}
	farFP, err := imageFingerprint(far)
	if err != nil {
		t.Fatalf("imageFingerprint(far) err = %v", err)
	}
	if visualSimilarity(baseFP, nearFP) <= visualSimilarity(baseFP, farFP) {
		t.Fatalf("near score <= far score: near=%f far=%f", visualSimilarity(baseFP, nearFP), visualSimilarity(baseFP, farFP))
	}
}

func TestImageFingerprintsIncludeTrimmedWhiteBorderVariant(t *testing.T) {
	base := testPosterPNG(color.RGBA{R: 10, G: 20, B: 200, A: 255}, color.RGBA{R: 240, G: 220, B: 20, A: 255})
	bordered := testPosterPNGWithBorder(color.RGBA{R: 10, G: 20, B: 200, A: 255}, color.RGBA{R: 240, G: 220, B: 20, A: 255}, color.White)
	baseFPs, err := imageFingerprints(base)
	if err != nil {
		t.Fatalf("imageFingerprints(base) err = %v", err)
	}
	borderedFPs, err := imageFingerprints(bordered)
	if err != nil {
		t.Fatalf("imageFingerprints(bordered) err = %v", err)
	}
	baseOnly, _ := imageFingerprint(base)
	borderedOnly, _ := imageFingerprint(bordered)
	if maxVisualSimilarity(baseFPs, borderedFPs) <= visualSimilarity(baseOnly, borderedOnly) {
		t.Fatalf("trimmed variants did not improve border match: variants=%f original=%f", maxVisualSimilarity(baseFPs, borderedFPs), visualSimilarity(baseOnly, borderedOnly))
	}
}

func TestImageFingerprintsTrimLargeWhiteBorderVariant(t *testing.T) {
	base := testPosterPNG(color.RGBA{R: 30, G: 35, B: 170, A: 255}, color.RGBA{R: 230, G: 150, B: 40, A: 255})
	bordered := testPosterPNGWithCustomBorder(64, 84, 16, 18, color.RGBA{R: 30, G: 35, B: 170, A: 255}, color.RGBA{R: 230, G: 150, B: 40, A: 255}, color.RGBA{R: 245, G: 244, B: 238, A: 255})
	baseFPs, err := imageFingerprints(base)
	if err != nil {
		t.Fatalf("imageFingerprints(base) err = %v", err)
	}
	borderedFPs, err := imageFingerprints(bordered)
	if err != nil {
		t.Fatalf("imageFingerprints(bordered) err = %v", err)
	}
	baseOnly, _ := imageFingerprint(base)
	borderedOnly, _ := imageFingerprint(bordered)
	variantScore := maxVisualSimilarity(baseFPs, borderedFPs)
	originalScore := visualSimilarity(baseOnly, borderedOnly)
	if variantScore < 0.95 || variantScore <= originalScore+0.20 {
		t.Fatalf("large border trim weak: variants=%f original=%f", variantScore, originalScore)
	}
}

func TestVisualSimilarityToleratesCropDiffNoise(t *testing.T) {
	left := visualFingerprint{width: 220, height: 334, lumaStdDev: 0.30}
	right := visualFingerprint{width: 434, height: 719, lumaStdDev: 0.432}
	setBoolSimilarity(left.avgHash[:], right.avgHash[:], 223)
	setBoolSimilarity(left.diffHash[:], right.diffHash[:], 145)
	left.colorHist[0], left.colorHist[1] = 0.85, 0.15
	right.colorHist[0], right.colorHist[2] = 0.85, 0.15
	if score := visualSimilarity(left, right); score < minVisualMatchScore {
		t.Fatalf("visualSimilarity() = %.4f, want confident match", score)
	}
}

func setBoolSimilarity(left, right []bool, same int) {
	for i := range left {
		left[i] = i%2 == 0
		if i < same {
			right[i] = left[i]
		} else {
			right[i] = !left[i]
		}
	}
}

func TestImageFingerprintDecodesJPEG(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 32, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 3), B: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode() err = %v", err)
	}
	if _, err := imageFingerprint(buf.Bytes()); err != nil {
		t.Fatalf("imageFingerprint() JPEG err = %v", err)
	}
}

func testPosterPNG(left, right color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 32; x++ {
			c := left
			if x > y/2 {
				c = right
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func testPosterPNGWithBorder(left, right, border color.Color) []byte {
	return testPosterPNGWithCustomBorder(40, 56, 4, 4, left, right, border)
}

func testPosterPNGWithCustomBorder(width, height, xBorder, yBorder int, left, right, border color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := border
			if x >= xBorder && x < width-xBorder && y >= yBorder && y < height-yBorder {
				innerX, innerY := x-xBorder, y-yBorder
				c = left
				if innerX > innerY/2 {
					c = right
				}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestDescriptiveTokensIgnoreTitleAndPosterNoise(t *testing.T) {
	t.Parallel()

	tokens := descriptiveTokens("Alien regular theatrical release poster 1979 jpg", plex.Movie{Title: "Alien", Year: 1979})
	if !tokens["regular"] {
		t.Fatalf("tokens = %#v, want regular", tokens)
	}
	for _, ignored := range []string{"alien", "theatrical", "release", "poster", "1979", "jpg"} {
		if tokens[ignored] {
			t.Fatalf("tokens[%q] = true, want ignored; tokens = %#v", ignored, tokens)
		}
	}
}
