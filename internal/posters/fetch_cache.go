package posters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) fetchText(ctx context.Context, rawURL string) (string, error) {
	if !forceRefresh(ctx) {
		if data, ok := s.readCache("text", rawURL); ok {
			return string(data), nil
		}
		if s.hasFreshNegativeCache(rawURL) {
			return "", fmt.Errorf("not found: %s", rawURL)
		}
	}
	key := "text:" + rawURL
	if forceRefresh(ctx) {
		key = "text-force:" + rawURL
	}
	value, err, _ := s.group.Do(key, func() (any, error) {
		return s.fetchTextUncached(ctx, rawURL)
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func (s *Service) fetchTextUncached(ctx context.Context, rawURL string) (string, error) {
	if err := s.throttle(ctx, rawURL); err != nil {
		return "", err
	}
	defer s.releaseThrottle(rawURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "posters/0.1 (+https://github.com/win0na/posters)")
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		s.writeCache("negative", rawURL, []byte(time.Now().Format(time.RFC3339Nano)))
		return "", fmt.Errorf("not found: %s", rawURL)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch %s: %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	s.writeCache("text", rawURL, data)
	return string(data), nil
}

func (s *Service) downloadIMPImage(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Host != "www.impawards.com" && u.Host != "impawards.com" {
		return nil, fmt.Errorf("refusing non-IMP image source: %s", rawURL)
	}
	return s.downloadImage(ctx, rawURL)
}

func (s *Service) downloadWikipediaImage(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported wikipedia image URL: %s", rawURL)
	}
	return s.downloadImage(ctx, rawURL)
}

func (s *Service) downloadImage(ctx context.Context, rawURL string) ([]byte, error) {
	if !forceRefresh(ctx) {
		if data, ok := s.readCache("images", rawURL); ok {
			return data, nil
		}
	}
	key := "image:" + rawURL
	if forceRefresh(ctx) {
		key = "image-force:" + rawURL
	}
	value, err, _ := s.group.Do(key, func() (any, error) {
		return s.downloadImageUncached(ctx, rawURL)
	})
	if err != nil {
		return nil, err
	}
	return value.([]byte), nil
}

func (s *Service) downloadImageUncached(ctx context.Context, rawURL string) ([]byte, error) {
	if err := s.throttle(ctx, rawURL); err != nil {
		return nil, err
	}
	defer s.releaseThrottle(rawURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "posters/0.1 (+https://github.com/win0na/posters)")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download poster: %s", resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("poster response is not image content: %s", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPosterSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPosterSize {
		return nil, fmt.Errorf("poster image too large")
	}
	s.writeCache("images", rawURL, data)
	return data, nil
}

func defaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "posters")
}

func (s *Service) cachePath(kind, rawURL string) string {
	if s.cacheDir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(rawURL))
	return filepath.Join(s.cacheDir, kind, hex.EncodeToString(sum[:]))
}

func (s *Service) readCache(kind, rawURL string) ([]byte, bool) {
	path := s.cachePath(kind, rawURL)
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil
}

func (s *Service) writeCache(kind, rawURL string, data []byte) {
	path := s.cachePath(kind, rawURL)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func (s *Service) hasFreshNegativeCache(rawURL string) bool {
	data, ok := s.readCache("negative", rawURL)
	if !ok {
		return false
	}
	created, err := time.Parse(time.RFC3339Nano, string(data))
	if err != nil {
		return true
	}
	return time.Since(created) < negativeCacheTTL
}

func (s *Service) getLimiter(host string) *hostLimiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.limiters == nil {
		s.limiters = map[string]*hostLimiter{}
	}
	if _, ok := s.limiters[host]; !ok {
		s.limiters[host] = newHostLimiter(hostMaxConcurrent(host))
	}
	return s.limiters[host]
}

func (s *Service) throttle(ctx context.Context, rawURL string) error {
	host := "default"
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	limiter := s.getLimiter(host)
	select {
	case limiter.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) releaseThrottle(rawURL string) {
	host := "default"
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	limiter := s.getLimiter(host)
	<-limiter.sem
}
