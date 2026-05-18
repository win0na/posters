package plex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/win0na/posters/internal/config"
)

const (
	product          = "posters"
	version          = "0.1.0"
	plexTV           = "https://plex.tv"
	moviePageSize    = 100
	maxMoviePageLoop = 10000
)

var ErrUnauthorized = errors.New("plex unauthorized")

type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
	Op         string
}

func (e *HTTPError) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("%s: %s: %s", e.Op, e.Status, e.Body)
	}
	return fmt.Sprintf("%s: %s", e.Status, e.Body)
}

func (e *HTTPError) Is(target error) bool {
	return target == ErrUnauthorized && (e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden)
}

func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

type Client struct {
	store *config.Store
	http  *http.Client
}

type Pin struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
}

type Server struct {
	Name        string
	ClientID    string
	AccessToken string
	URI         string
}

type Library struct {
	Key   string
	Title string
	Type  string
}

type Movie struct {
	RatingKey string
	Title     string
	Year      int
	GUID      string
}

func NewClient(store *config.Store) *Client {
	return &Client{store: store, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) StartPIN(ctx context.Context) (Pin, string, error) {
	state, err := c.store.LoadState()
	if err != nil {
		return Pin{}, "", err
	}
	form := url.Values{"strong": {"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, plexTV+"/api/v2/pins", strings.NewReader(form.Encode()))
	if err != nil {
		return Pin{}, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.setPlexTVHeaders(req, state.ClientID)

	var pin Pin
	if err := c.doJSON(req, &pin); err != nil {
		return Pin{}, "", err
	}
	authURL := fmt.Sprintf("https://app.plex.tv/auth#?clientID=%s&code=%s&context[device][product]=%s", url.QueryEscape(state.ClientID), url.QueryEscape(pin.Code), url.QueryEscape(product))
	return pin, authURL, nil
}

func (c *Client) PollPIN(ctx context.Context, pinID int) (string, error) {
	state, err := c.store.LoadState()
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v2/pins/%d", plexTV, pinID), nil)
	if err != nil {
		return "", err
	}
	c.setPlexTVHeaders(req, state.ClientID)

	var pin Pin
	if err := c.doJSON(req, &pin); err != nil {
		return "", err
	}
	if pin.AuthToken == "" {
		return "", nil
	}
	state.PlexToken = pin.AuthToken
	if err := c.store.SaveState(state); err != nil {
		return "", err
	}
	return pin.AuthToken, nil
}

func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	state, err := c.store.LoadState()
	if err != nil {
		return nil, err
	}
	if state.PlexToken == "" {
		return nil, fmt.Errorf("not logged in")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, plexTV+"/api/v2/resources?includeHttps=1&includeRelay=1", nil)
	if err != nil {
		return nil, err
	}
	c.setPlexTVHeaders(req, state.ClientID)
	req.Header.Set("X-Plex-Token", state.PlexToken)

	var raw []struct {
		Name        string `json:"name"`
		ClientID    string `json:"clientIdentifier"`
		Provides    string `json:"provides"`
		AccessToken string `json:"accessToken"`
		Connections []struct {
			URI   string `json:"uri"`
			Local bool   `json:"local"`
		} `json:"connections"`
	}
	if err := c.doJSON(req, &raw); err != nil {
		return nil, err
	}

	servers := make([]Server, 0, len(raw))
	for _, resource := range raw {
		if !strings.Contains(resource.Provides, "server") || len(resource.Connections) == 0 {
			continue
		}
		uri := resource.Connections[0].URI
		for _, connection := range resource.Connections {
			if connection.Local {
				uri = connection.URI
				break
			}
		}
		servers = append(servers, Server{Name: resource.Name, ClientID: resource.ClientID, AccessToken: resource.AccessToken, URI: uri})
	}
	return servers, nil
}

func (c *Client) ListLibraries(ctx context.Context, server Server) ([]Library, error) {
	var out struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := c.getPMS(ctx, server, "/library/sections", &out); err != nil {
		return nil, err
	}
	libraries := make([]Library, 0, len(out.MediaContainer.Directory))
	for _, directory := range out.MediaContainer.Directory {
		if directory.Type == "movie" {
			libraries = append(libraries, Library{Key: directory.Key, Title: directory.Title, Type: directory.Type})
		}
	}
	return libraries, nil
}

func (c *Client) ListMovies(ctx context.Context, server Server, library Library) ([]Movie, error) {
	movies := []Movie{}
	for start := 0; start < maxMoviePageLoop; start += moviePageSize {
		page, err := c.listMoviePage(ctx, server, library, start, moviePageSize)
		if err != nil {
			return nil, err
		}
		movies = append(movies, page.Movies...)
		if len(page.Movies) == 0 || page.Size == 0 || len(movies) >= page.Size || len(page.Movies) < moviePageSize {
			return movies, nil
		}
	}
	return nil, fmt.Errorf("movie listing exceeded pagination safety limit")
}

type moviePage struct {
	Size   int
	Movies []Movie
}

func (c *Client) listMoviePage(ctx context.Context, server Server, library Library, start, size int) (moviePage, error) {
	var out struct {
		MediaContainer struct {
			Size      int `json:"size"`
			TotalSize int `json:"totalSize"`
			Metadata  []struct {
				RatingKey string `json:"ratingKey"`
				Title     string `json:"title"`
				Year      int    `json:"year"`
				GUID      string `json:"guid"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	path := fmt.Sprintf("/library/sections/%s/all", url.PathEscape(library.Key))
	query := url.Values{
		"X-Plex-Container-Start": {strconv.Itoa(start)},
		"X-Plex-Container-Size":  {strconv.Itoa(size)},
	}
	if err := c.getPMS(ctx, server, path+"?"+query.Encode(), &out); err != nil {
		return moviePage{}, err
	}
	movies := make([]Movie, 0, len(out.MediaContainer.Metadata))
	for _, item := range out.MediaContainer.Metadata {
		movies = append(movies, Movie{RatingKey: item.RatingKey, Title: item.Title, Year: item.Year, GUID: item.GUID})
	}
	total := out.MediaContainer.TotalSize
	if total == 0 {
		total = out.MediaContainer.Size
	}
	if total == 0 && len(movies) > 0 {
		total = len(movies)
	}
	return moviePage{Size: total, Movies: movies}, nil
}

func (c *Client) UploadPoster(ctx context.Context, server Server, movie Movie, filename string, data []byte, sourceURL string) error {
	var attempts []error

	for _, path := range []string{
		"/library/metadata/" + url.PathEscape(movie.RatingKey) + "/posters",
		"/library/metadata/" + url.PathEscape(movie.RatingKey) + "/poster",
	} {
		if err := c.uploadPosterMultipart(ctx, server, path, filename, data); err == nil {
			return nil
		} else {
			attempts = append(attempts, err)
		}
	}

	if sourceURL != "" {
		for _, attempt := range []struct {
			method string
			path   string
		}{
			{method: http.MethodPost, path: "/library/metadata/" + url.PathEscape(movie.RatingKey) + "/posters"},
			{method: http.MethodPut, path: "/library/metadata/" + url.PathEscape(movie.RatingKey) + "/poster"},
		} {
			if err := c.uploadPosterURL(ctx, server, attempt.method, attempt.path, sourceURL); err == nil {
				return nil
			} else {
				attempts = append(attempts, err)
			}
		}
	}

	return fmt.Errorf("upload poster failed after %d attempts; PMS artwork endpoint may differ by server version; run a one-movie smoke test and inspect these endpoint responses: %w", len(attempts), errors.Join(attempts...))
}

func (c *Client) uploadPosterMultipart(ctx context.Context, server Server, path, filename string, data []byte) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	endpoint := strings.TrimRight(server.URI, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.setPMSHeaders(req, server)
	return c.doNoContent(req, "upload poster multipart "+path)
}

func (c *Client) uploadPosterURL(ctx context.Context, server Server, method, path, sourceURL string) error {
	endpoint, err := url.Parse(strings.TrimRight(server.URI, "/") + path)
	if err != nil {
		return err
	}
	query := endpoint.Query()
	query.Set("url", sourceURL)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), nil)
	if err != nil {
		return err
	}
	c.setPMSHeaders(req, server)
	return c.doNoContent(req, "upload poster url "+method+" "+path)
}

func (c *Client) getPMS(ctx context.Context, server Server, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(server.URI, "/")+path, nil)
	if err != nil {
		return err
	}
	c.setPMSHeaders(req, server)
	return c.doJSON(req, out)
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(body))}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) doNoContent(req *http.Request, label string) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(body)), Op: label}
	}
	return nil
}

func (c *Client) setPlexTVHeaders(req *http.Request, clientID string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", product)
	req.Header.Set("X-Plex-Version", version)
	req.Header.Set("X-Plex-Client-Identifier", clientID)
	req.Header.Set("X-Plex-Platform", "Go")
	req.Header.Set("X-Plex-Device", "CLI")
	req.Header.Set("X-Plex-Device-Name", product)
}

func (c *Client) setPMSHeaders(req *http.Request, server Server) {
	state, _ := c.store.LoadState()
	c.setPlexTVHeaders(req, state.ClientID)
	token := server.AccessToken
	if token == "" {
		token = state.PlexToken
	}
	req.Header.Set("X-Plex-Token", token)
}
