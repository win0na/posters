package posters

import (
	"fmt"
	"html"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/win0na/posters/internal/plex"
)

func impProbeURLs(movie plex.Movie) []string {
	return impProbeURLsForYear(movie, movie.Year)
}

func impProbeURLsForYear(movie plex.Movie, year int) []string {
	urls := impCanonicalProbeURLsForYear(movie, year)
	urls = append(urls, impVersionProbeURLsForYear(movie, year)...)
	urls = append(urls, impShortenedCanonicalProbeURLsForYear(movie, year)...)
	return urls
}

func impCanonicalProbeURLsForYear(movie plex.Movie, year int) []string {
	urls := []string{}
	for _, slug := range titleSlugs(movie.Title) {
		base := fmt.Sprintf("%s/%d/%s", impBase, year, slug)
		urls = append(urls, base+".html")
	}
	return urls
}

func impShortenedCanonicalProbeURLsForYear(movie plex.Movie, year int) []string {
	urls := []string{}
	seen := map[string]bool{}
	for _, slug := range titleSlugs(movie.Title) {
		parts := strings.Split(slug, "_")
		for len(parts) > 1 {
			parts = parts[:len(parts)-1]
			shortSlug := strings.Join(parts, "_")
			if seen[shortSlug] {
				continue
			}
			seen[shortSlug] = true
			urls = append(urls, fmt.Sprintf("%s/%d/%s.html", impBase, year, shortSlug))
		}
	}
	return urls
}

func impVersionProbeURLsForYear(movie plex.Movie, year int) []string {
	urls := []string{}
	for _, slug := range titleSlugs(movie.Title) {
		base := fmt.Sprintf("%s/%d/%s", impBase, year, slug)
		for version := 1; version <= 8; version++ {
			urls = append(urls, fmt.Sprintf("%s_ver%d.html", base, version))
		}
	}
	return urls
}

func versionURL(pageURL string, version int) string {
	return fmt.Sprintf("%s_ver%d.html", strings.TrimSuffix(pageURL, ".html"), version)
}

func isFullTitleSlug(candidateURL string, movieTitle string) bool {
	slug := strings.TrimSuffix(path.Base(candidateURL), ".html")
	for _, titleSlug := range titleSlugs(movieTitle) {
		if slug == titleSlug {
			return true
		}
	}
	return false
}

func impCandidateYears(year int) []int {
	if year <= 0 {
		return nil
	}
	return []int{year, year - 1, year + 1, year - 2, year + 2, year - 3, year + 3}
}

func titleSlugs(title string) []string {
	normal := normalizeTitle(title)
	parts := strings.Fields(normal)
	if len(parts) == 0 {
		return nil
	}
	seen := map[string]bool{}
	slugs := []string{}
	for _, variant := range titleSlugPartVariants(parts) {
		for _, slugParts := range articleSlugPartVariants(variant) {
			slug := strings.Join(slugParts, "_")
			if !seen[slug] {
				seen[slug] = true
				slugs = append(slugs, slug)
			}
		}
	}
	return slugs
}

func titleSlugPartVariants(parts []string) [][]string {
	variants := [][]string{append([]string(nil), parts...)}
	numberWords := map[string]string{
		"0":   "zero",
		"1":   "one",
		"2":   "two",
		"3":   "three",
		"4":   "four",
		"5":   "five",
		"6":   "six",
		"7":   "seven",
		"8":   "eight",
		"9":   "nine",
		"i":   "one",
		"ii":  "two",
		"iii": "three",
		"iv":  "four",
		"v":   "five",
		"vi":  "six",
	}
	replaced := append([]string(nil), parts...)
	changed := false
	for i, part := range replaced {
		if word, ok := numberWords[part]; ok {
			replaced[i] = word
			changed = true
		}
	}
	if changed {
		variants = append(variants, replaced)
	}
	if len(replaced) > 4 && replaced[0] == "star" && replaced[1] == "wars" && replaced[2] == "episode" {
		variants = append(variants, append([]string(nil), replaced[:4]...))
	}
	if len(parts) >= 2 && parts[len(parts)-1] == "movie" && parts[len(parts)-2] != "the" {
		withThe := append([]string(nil), parts[:len(parts)-1]...)
		withThe = append(withThe, "the", "movie")
		variants = append(variants, withThe)
	}
	return variants
}

func titleMatches(movieTitle, candidateTitle string) bool {
	movie := comparableTitle(movieTitle)
	candidate := comparableTitle(candidateTitle)
	if movie == "" || candidate == "" {
		return false
	}
	if movie == candidate {
		return true
	}
	movieTokens := strings.Fields(movie)
	candidateTokens := strings.Fields(candidate)

	// movie (≥2 tokens) is a prefix of a longer candidate title
	// e.g. "Glass Onion" + "Glass Onion: A Knives Out Mystery"
	if len(movieTokens) >= 2 && len(candidateTokens) > len(movieTokens) {
		matchesPrefix := true
		for i, token := range movieTokens {
			if candidateTokens[i] != token {
				matchesPrefix = false
				break
			}
		}
		if matchesPrefix {
			return true
		}
	}

	// candidate is a prefix of a longer movie title
	// e.g. "Furiosa: A Mad Max Saga" + IMP heading "Furiosa"
	// Require candidate >= 2 tokens or movie >= 3 tokens to avoid
	// false matches from short movie titles with common words.
	// "Glass Onion" + IMP heading "Glass" would match (bad).
	// "Furiosa: A Mad Max Saga" + IMP heading "Furiosa" would match (good).
	if len(movieTokens) > len(candidateTokens) {
		if len(candidateTokens) < 2 && len(movieTokens) < 3 {
			return false
		}
		matchesPrefix := true
		for i, token := range candidateTokens {
			if movieTokens[i] != token {
				matchesPrefix = false
				break
			}
		}
		if matchesPrefix {
			return true
		}
	}
	return false
}

func comparableTitle(title string) string {
	parts := strings.Fields(normalizeTitle(title))
	for i, part := range parts {
		if replacement, ok := comparableNumberTokens[part]; ok {
			parts[i] = replacement
		}
	}
	return strings.Join(parts, " ")
}

var comparableNumberTokens = map[string]string{
	"1": "one", "2": "two", "3": "three", "4": "four", "5": "five", "6": "six",
	"i": "one", "ii": "two", "iii": "three", "iv": "four", "v": "five", "vi": "six",
}

func articleSlugPartVariants(parts []string) [][]string {
	variants := [][]string{append([]string(nil), parts...)}
	if len(parts) > 1 && (parts[0] == "the" || parts[0] == "a" || parts[0] == "an") {
		moved := append([]string(nil), parts[1:]...)
		moved = append(moved, parts[0])
		variants = append(variants, moved)
		// Also drop the article entirely — IMP often omits leading articles
		// in slugs (e.g. "The Empire Strikes Back" → "empire_strikes_back").
		dropped := parts[1:]
		variants = append(variants, dropped)
	}
	return variants
}

func normalizeTitle(title string) string {
	title = strings.ToLower(title)
	var b strings.Builder
	for _, r := range title {
		r = foldTitleRune(r)
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == ':' || r == '&' || r == '/' || r == '.':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func foldTitleRune(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ä', 'ã', 'å', 'ā':
		return 'a'
	case 'ç':
		return 'c'
	case 'é', 'è', 'ê', 'ë', 'ē':
		return 'e'
	case 'í', 'ì', 'î', 'ï', 'ī':
		return 'i'
	case 'ñ':
		return 'n'
	case 'ó', 'ò', 'ô', 'ö', 'õ', 'ō':
		return 'o'
	case 'ú', 'ù', 'û', 'ü', 'ū':
		return 'u'
	case 'ý', 'ÿ':
		return 'y'
	}
	return r
}

func cleanText(text string) string {
	text = tagRE.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return strings.Join(strings.Fields(text), " ")
}

func absoluteURL(baseURL, raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

func normalizeWikiImageURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

func wikipediaOriginalImageURL(raw string) string {
	u, err := url.Parse(normalizeWikiImageURL(raw))
	if err != nil {
		return normalizeWikiImageURL(raw)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	thumb := -1
	for i, part := range parts {
		if part == "thumb" {
			thumb = i
			break
		}
	}
	if thumb == -1 || len(parts) < thumb+5 {
		return u.String()
	}
	parts = append(parts[:thumb], parts[thumb+1:len(parts)-1]...)
	u.Path = "/" + strings.Join(parts, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func versionFromURL(rawURL string) int {
	base := path.Base(rawURL)
	start := strings.Index(base, "_ver")
	if start == -1 {
		return 0
	}
	start += len("_ver")
	end := start
	for end < len(base) && base[end] >= '0' && base[end] <= '9' {
		end++
	}
	version, _ := strconv.Atoi(base[start:end])
	return version
}

func imageRank(rawURL string) int {
	score := 0
	if strings.Contains(rawURL, "/posters/") {
		score += 10
	}
	if strings.Contains(rawURL, "_xxlg") {
		score += 4
	}
	if strings.Contains(rawURL, "_xlg") {
		score += 3
	}
	return score
}
