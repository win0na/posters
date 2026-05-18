# posters

Go TUI for updating Plex movie posters to original theatrical posters.

## status

Early scaffold. Current build includes:

- Bubble Tea login/server/library/mode/movie/progress flow
- Plex PIN auth request and polling
- Plex server, movie library, and movie listing clients
- local credentials and poster metadata under `~/.config/posters`
- IMP Awards discovery/search plus Wikipedia poster confirmation

## run

```sh
go run ./cmd/posters
```

Useful flags:

```sh
go run ./cmd/posters -dry-run
go run ./cmd/posters -force
go run ./cmd/posters -config-dir /tmp/posters-smoke
go run ./cmd/posters -version
```

Release/dev helpers:

```sh
make test
make smoke-dry-run
make install
```

## verify

```sh
go test ./...
```

## behavior

- no environment variables or API keys are required
- Plex credentials are saved in `~/.config/posters/state.json`
- poster update metadata is saved locally in `~/.config/posters/metadata.json`
- poster assets must come only from IMP Awards
- Wikipedia is used only to confirm the theatrical poster
- ambiguous poster matches are skipped
- run results show updated, dry-run, skipped, ambiguous, and failed counts
- each run writes JSON and CSV reports under `~/.config/posters/reports`
- Wikipedia/IMP page and image fetches are cached under the user cache directory
- movies already recorded in local metadata are skipped by default
- press `f` on update-mode or movie-selection screens to force refresh already-recorded movies
- press `d` on update-mode or movie-selection screens to dry-run matches without uploading or recording metadata
- press `s` to show config/status details
- press `Esc` while loading, waiting for Plex login, or updating to cancel in-flight work
- expired or rejected Plex auth clears the saved token and returns to login; press `r` on error/login screens to clear saved login manually
- last selected Plex server and library are saved and preselected on later runs
- poster updates run sequentially to keep Plex and poster-source traffic polite

## manual Plex smoke test

Use a tiny test run before updating a full library.

1. Start isolated if desired:

   ```sh
   go run ./cmd/posters -config-dir /tmp/posters-smoke -dry-run
   ```

2. Log in with the Plex PIN URL/code.
3. Choose the target Plex server and movie library.
4. Keep dry-run enabled (`d`) and choose **Specific posters**.
5. Select one low-risk movie, run it, and inspect:
   - chosen IMP Awards page URL
   - IMP image URL
   - match reason
   - no upload happened and no metadata was recorded
6. Restart without `-dry-run`, select the same single movie, and run one real upload.
7. Confirm in Plex that the poster changed as expected.
8. If wrong, use Plex's poster picker/edit UI to restore a previous poster manually.
9. Only after the single-poster test succeeds, run a larger dry-run, then a larger real run.

Useful files:

- state/token/last selection: `~/.config/posters/state.json`
- completion metadata: `~/.config/posters/metadata.json`
- run reports: `~/.config/posters/reports/`
- isolated smoke-test state when using `-config-dir`: that directory's `state.json` and `metadata.json`

## local metadata reset

Poster completion state lives in:

```sh
~/.config/posters/metadata.json
```

Delete that file to make all movies eligible again, or use force refresh in the TUI for a one-off rerun.
