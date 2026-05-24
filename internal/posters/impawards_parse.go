package posters

import (
	"html"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

func parseIMPCandidate(pageURL, body string) (impCandidate, bool) {
	title, year, ok := parseIMPHeading(body)
	if !ok {
		return impCandidate{}, false
	}
	imageURL := bestIMPImage(pageURL, body)
	if imageURL == "" {
		return impCandidate{}, false
	}
	version := versionFromURL(pageURL)
	return impCandidate{Title: title, Year: year, PageURL: pageURL, ImageURL: imageURL, Version: version, Canonical: !strings.Contains(path.Base(pageURL), "_ver")}, true
}

func parseIMPSearchResults(baseURL, body string) []string {
	matches := impLinkRE.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	urls := []string{}
	for _, match := range matches {
		raw := html.UnescapeString(match[1])
		pageURL := absoluteURL(baseURL, raw)
		if seen[pageURL] || !isIMPURL(pageURL) {
			continue
		}
		if !strings.HasSuffix(pageURL, ".html") || strings.Contains(pageURL, "_gallery") || strings.Contains(pageURL, "/news/") {
			continue
		}
		seen[pageURL] = true
		urls = append(urls, pageURL)
	}
	return urls
}

func isIMPURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Host == "www.impawards.com" || u.Host == "impawards.com"
}

func parseIMPPageLinksForTitle(baseURL, body, title string, year int) []string {
	seen := map[string]bool{}
	urls := []string{}
	for _, match := range impLinkRE.FindAllStringSubmatch(body, -1) {
		if len(match) < 3 {
			continue
		}
		pageURL := absoluteURL(baseURL, html.UnescapeString(match[1]))
		if seen[pageURL] || !looksLikeIMPMoviePage(pageURL, year) {
			continue
		}
		linkText := cleanText(match[2])
		if !titleMatches(title, linkText) {
			continue
		}
		seen[pageURL] = true
		urls = append(urls, pageURL)
	}
	return urls
}

func looksLikeIMPMoviePage(rawURL string, year int) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if !isIMPURL(rawURL) {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return false
	}
	yearText := strconv.Itoa(year)
	yearIndex := -1
	for i, part := range parts[:len(parts)-1] {
		if part == yearText {
			yearIndex = i
			break
		}
	}
	if yearIndex == -1 {
		return false
	}
	file := parts[len(parts)-1]
	return strings.HasSuffix(file, ".html") && !strings.Contains(file, "_gallery")
}

func parseIMPHeading(body string) (string, int, bool) {
	matches := impHeadingRE.FindStringSubmatch(body)
	if len(matches) > 0 {
		title, yearText := matches[1], matches[2]
		if title == "" {
			title, yearText = matches[3], matches[4]
		}
		title = cleanText(title)
		year, err := strconv.Atoi(yearText)
		if err == nil {
			return title, year, true
		}
	}
	for _, match := range impHRE.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}
		text := cleanText(match[1])
		parts := regexp.MustCompile(`^(.*?)\s*\(\s*(\d{4})\s*\)`).FindStringSubmatch(text)
		if len(parts) != 3 {
			continue
		}
		year, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		return strings.TrimSpace(parts[1]), year, true
	}
	return "", 0, false
}

func bestIMPImage(pageURL, body string) string {
	best := ""
	for _, match := range impImageRE.FindAllStringSubmatch(body, -1) {
		candidate := absoluteURL(pageURL, html.UnescapeString(match[1]))
		if best == "" || imageRank(candidate) > imageRank(best) {
			best = candidate
		}
	}
	for _, match := range impSizePageRE.FindAllStringSubmatch(body, -1) {
		candidate := imageURLFromIMPSizePage(pageURL, html.UnescapeString(match[1]))
		if candidate == "" {
			continue
		}
		if best == "" || imageRank(candidate) > imageRank(best) {
			best = candidate
		}
	}
	if upgraded := upgradeIMPImageFromSizeLinks(pageURL, best, body); upgraded != "" && imageRank(upgraded) > imageRank(best) {
		best = upgraded
	}
	return best
}

func upgradeIMPImageFromSizeLinks(pageURL, imageURL, body string) string {
	if imageURL == "" || strings.Contains(imageURL, "_xlg") || strings.Contains(imageURL, "_xxlg") {
		return ""
	}
	u, err := url.Parse(imageURL)
	if err != nil {
		return ""
	}
	base := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	pageDir := path.Dir(mustURLPath(pageURL))
	for _, suffix := range []string{"_xxlg", "_xlg"} {
		pageName := base + suffix + ".html"
		if !strings.Contains(body, pageName) {
			continue
		}
		upgraded := *u
		upgraded.Path = path.Join(pageDir, "posters", base+suffix+".jpg")
		upgraded.RawQuery = ""
		upgraded.Fragment = ""
		return upgraded.String()
	}
	return ""
}

func mustURLPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Path
}

func imageURLFromIMPSizePage(pageURL, raw string) string {
	sizePage := absoluteURL(pageURL, raw)
	u, err := url.Parse(sizePage)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if !strings.HasSuffix(base, ".html") {
		return ""
	}
	imageBase := strings.TrimSuffix(base, ".html") + ".jpg"
	u.Path = path.Join(path.Dir(u.Path), "posters", imageBase)
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func parseWikipediaPoster(title, body string) wikiPoster {
	poster := wikiPoster{PageTitle: title}
	matches := wikiImgRE.FindStringSubmatch(body)
	if len(matches) >= 2 {
		poster.ImageURL = normalizeWikiImageURL(matches[1])
	}
	if len(matches) >= 3 {
		poster.Alt = cleanText(matches[2])
	}
	infobox := wikiCapRE.FindString(body)
	poster.Caption = cleanText(infobox)
	signal := strings.ToLower(poster.ImageURL + " " + poster.Alt + " " + poster.Caption)
	poster.Poster = strings.Contains(signal, "poster") || strings.Contains(signal, "one-sheet") || strings.Contains(signal, "one sheet") || strings.Contains(signal, "theatrical")
	return poster
}
