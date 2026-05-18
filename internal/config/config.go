package config

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	appDirName   = "posters"
	stateFile    = "state.json"
	metadataFile = "metadata.json"
)

type Store struct {
	dir        string
	metadataMu sync.Mutex
}

type State struct {
	ClientID         string    `json:"client_id"`
	PlexToken        string    `json:"plex_token,omitempty"`
	LastServerID     string    `json:"last_server_id,omitempty"`
	LastServerName   string    `json:"last_server_name,omitempty"`
	LastServerURI    string    `json:"last_server_uri,omitempty"`
	LastLibraryKey   string    `json:"last_library_key,omitempty"`
	LastLibraryTitle string    `json:"last_library_title,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PosterMetadata struct {
	Items map[string]PosterItem `json:"items"`
}

type PosterItem struct {
	RatingKey string    `json:"rating_key"`
	Title     string    `json:"title"`
	Year      int       `json:"year,omitempty"`
	SourceURL string    `json:"source_url"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ReportStats struct {
	Updated      int  `json:"updated"`
	DryRun       int  `json:"dry_run"`
	WikiFallback int  `json:"wiki_fallback"`
	Skipped      int  `json:"skipped"`
	Ambiguous    int  `json:"ambiguous"`
	Failed       int  `json:"failed"`
	Cancelled    bool `json:"cancelled"`
}

type ReportItem struct {
	RatingKey   string `json:"rating_key"`
	Title       string `json:"title"`
	Year        int    `json:"year,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	SourceURL   string `json:"source_url,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	MatchReason string `json:"match_reason,omitempty"`
	Error       string `json:"error,omitempty"`
}

type RunReport struct {
	StartedAt   time.Time    `json:"started_at"`
	CompletedAt time.Time    `json:"completed_at"`
	Stats       ReportStats  `json:"stats"`
	Items       []ReportItem `json:"items"`
}

func Open() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, appDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func OpenDir(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) LoadState() (State, error) {
	var state State
	if err := readJSON(filepath.Join(s.dir, stateFile), &state); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return State{}, err
		}
	}
	if state.ClientID == "" {
		id, err := randomHex(16)
		if err != nil {
			return State{}, err
		}
		state.ClientID = id
		state.UpdatedAt = time.Now()
		if err := s.SaveState(state); err != nil {
			return State{}, err
		}
	}
	return state, nil
}

func (s *Store) SaveState(state State) error {
	state.UpdatedAt = time.Now()
	return writeJSON(filepath.Join(s.dir, stateFile), state, 0o600)
}

func (s *Store) ClearPlexToken() error {
	state, err := s.LoadState()
	if err != nil {
		return err
	}
	state.PlexToken = ""
	return s.SaveState(state)
}

func (s *Store) SaveLastSelection(serverID, serverName, serverURI, libraryKey, libraryTitle string) error {
	state, err := s.LoadState()
	if err != nil {
		return err
	}
	state.LastServerID = serverID
	state.LastServerName = serverName
	state.LastServerURI = serverURI
	state.LastLibraryKey = libraryKey
	state.LastLibraryTitle = libraryTitle
	return s.SaveState(state)
}

func (s *Store) LoadMetadata() (PosterMetadata, error) {
	s.metadataMu.Lock()
	defer s.metadataMu.Unlock()
	return s.loadMetadataUnlocked()
}

func (s *Store) loadMetadataUnlocked() (PosterMetadata, error) {
	metadata := PosterMetadata{Items: map[string]PosterItem{}}
	if err := readJSON(filepath.Join(s.dir, metadataFile), &metadata); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return metadata, nil
		}
		return PosterMetadata{}, err
	}
	if metadata.Items == nil {
		metadata.Items = map[string]PosterItem{}
	}
	return metadata, nil
}

func (s *Store) MarkPosterUpdated(item PosterItem) error {
	s.metadataMu.Lock()
	defer s.metadataMu.Unlock()
	metadata, err := s.loadMetadataUnlocked()
	if err != nil {
		return err
	}
	item.UpdatedAt = time.Now()
	metadata.Items[item.RatingKey] = item
	return writeJSON(filepath.Join(s.dir, metadataFile), metadata, 0o600)
}

func (s *Store) PosterUpdated(ratingKey string) (bool, error) {
	s.metadataMu.Lock()
	defer s.metadataMu.Unlock()
	metadata, err := s.loadMetadataUnlocked()
	if err != nil {
		return false, err
	}
	_, ok := metadata.Items[ratingKey]
	return ok, nil
}

func (s *Store) SaveRunReport(report RunReport) (string, string, error) {
	now := time.Now()
	if report.StartedAt.IsZero() {
		report.StartedAt = now
	}
	report.CompletedAt = now
	dir := filepath.Join(s.dir, "reports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	stamp := now.Format("20060102-150405")
	jsonPath := filepath.Join(dir, "run-"+stamp+".json")
	csvPath := filepath.Join(dir, "run-"+stamp+".csv")
	if err := writeJSON(jsonPath, report, 0o600); err != nil {
		return "", "", err
	}
	if err := writeReportCSV(csvPath, report.Items); err != nil {
		return "", "", err
	}
	return jsonPath, csvPath, nil
}

func writeReportCSV(path string, items []ReportItem) error {
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := csv.NewWriter(file)
	rows := [][]string{{"rating_key", "title", "year", "status", "message", "source_url", "image_url", "match_reason", "error"}}
	for _, item := range items {
		rows = append(rows, []string{item.RatingKey, item.Title, strconv.Itoa(item.Year), item.Status, item.Message, item.SourceURL, item.ImageURL, item.MatchReason, item.Error})
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			_ = file.Close()
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func writeJSON(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random client id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
