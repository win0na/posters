<div align="center">

# posters

a Go TUI for setting Plex movie libraries back to original theatrical posters.

Plex auth. IMP Awards discovery. Wikipedia confirmation. local completion state.

<p>
  <a href="#quickstart">quickstart</a> •
  <a href="#how-it-works">how it works</a> •
  <a href="#common-commands">common commands</a> •
  <a href="#smoke-test">smoke test</a>
</p>

</div>

## what it does

`posters` logs into Plex, scans a movie library, finds theatrical poster candidates on [IMP Awards](http://www.impawards.com), verifies likely matches against the movie's Wikipedia infobox poster, and uploads the chosen image back to Plex.

It stores credentials and completion metadata locally, so later runs skip movies already updated unless force mode is enabled.

| area | behavior |
|------|----------|
| auth | Plex PIN login; no env vars or API keys |
| source | IMP Awards is primary poster source |
| confirmation | Wikipedia main poster is used for visual confirmation |
| fallback | Wikipedia image upload is opt-in with `-wiki-fallback` or `w` |
| state | token, last server/library, metadata, reports under `~/.config/posters` |
| UI | Bubble Tea TUI with cancellable async auth, selection, matching, and updates |

## quickstart

```sh
go run ./cmd/posters
```

then:

1. open the Plex auth URL and enter the PIN code
2. choose a Plex server
3. choose a movie library
4. choose all eligible posters or specific posters
5. review progress and report output

### safer first run

```sh
go run ./cmd/posters -dry-run
```

### isolated smoke run

```sh
go run ./cmd/posters -config-dir /tmp/posters-smoke -dry-run
```

## flags

```sh
go run ./cmd/posters -dry-run        # match only; no upload and no metadata write
go run ./cmd/posters -force          # include movies already recorded as updated
go run ./cmd/posters -wiki-fallback  # use Wikipedia poster when IMP is missing/ambiguous
go run ./cmd/posters -config-dir DIR # use alternate state/metadata directory
go run ./cmd/posters -version        # print version
```

## TUI keys

| key | action |
|-----|--------|
| `enter` | choose / continue / start selected mode |
| `space` | toggle movie selection in specific-poster mode |
| `a` | choose all-posters mode |
| `p` | choose specific-posters mode |
| `d` | toggle dry-run on mode/movie screens |
| `f` | toggle force refresh on mode/movie screens |
| `w` | toggle Wikipedia fallback on mode/movie screens |
| `s` | show status/config screen |
| `esc` | back or cancel in-flight auth/update work |
| `r` | clear saved Plex login on login/error screens |
| `q`, `ctrl+c` | quit |

## how it works

### matching pipeline

1. Fetch Plex movies, including title, original title, year, GUID, and rating key.
2. Search IMP Awards by exact title probes first.
3. If Plex has a distinct `OriginalTitle`, probe that title before broad IMP search fallback.
4. Try adjacent release years when needed.
5. Follow relevant IMP search and nominee links while staying on `impawards.com`.
6. Fetch the Wikipedia page most likely to be the same film.
7. Compare IMP images to the Wikipedia infobox poster using visual fingerprints.
8. Prefer confident visual matches; mark unclear matches as ambiguous instead of guessing.

### visual matching

The matcher compares average hash, difference hash, color histogram, aspect ratio, and contrast. It also builds cropped variants for common poster-image differences:

- white or black border trim
- small 2% and 4% insets
- filename hint when Wikipedia and IMP share the same poster image stem

Example progress row:

```text
✓ updated The Empire Strikes Back (1980), visual match 87.8%
```

### update behavior

- default mode updates all eligible movies
- movies in local metadata are skipped unless force mode is on
- dry-run reports matches without upload or metadata writes
- ambiguous matches are reported as `ambiguous`
- missing IMP matches are reported as `skipped`
- real updates upload the selected image to Plex and record local completion metadata
- update workers run concurrently with polite throttling for Plex, IMP, and Wikipedia requests

## files

Default state directory:

```text
~/.config/posters
```

Important files:

| path | purpose |
|------|---------|
| `state.json` | Plex token, local client id, last server/library |
| `metadata.json` | local poster-update completion state |
| `reports/run-*.json` | run report with stats and per-movie details |
| `reports/run-*.csv` | CSV copy of each run report |

Fetched Wikipedia/IMP text and images are cached under the user cache directory for faster repeated matching.

## common commands

```sh
make run            # go run ./cmd/posters
make test           # go test ./...
make fmt            # gofmt -w cmd internal
make smoke-dry-run  # isolated dry-run under /tmp/posters-smoke
make install        # go install ./cmd/posters
```

Direct commands:

```sh
go test ./...
go test ./internal/posters
go test ./internal/tui
```

## smoke test

Use a tiny dry-run before updating a real library.

```sh
go run ./cmd/posters -config-dir /tmp/posters-smoke -dry-run
```

1. log in with the Plex PIN URL/code
2. select the target Plex server and movie library
3. keep dry-run enabled
4. choose specific-posters mode
5. select one low-risk movie
6. verify the chosen IMP page, image URL, and match reason in the TUI/report
7. rerun without `-dry-run` for the same single movie
8. confirm Plex shows the expected poster
9. only then run a larger dry-run, followed by a larger real run

If a poster is wrong, restore it manually through Plex's poster picker/edit UI, then rerun with force mode after fixing the local metadata if needed.

## reset local metadata

Completion state lives in:

```text
~/.config/posters/metadata.json
```

Delete that file to make all movies eligible again, or use force mode for a one-off rerun.

For an isolated reset, use `-config-dir` and remove that directory instead.

## notes

- `posters` never needs user-provided API keys.
- Plex tokens are stored locally with `0600` file permissions.
- Completion state is not written into Plex metadata.
- IMP Awards remains the primary poster source.
- Wikipedia fallback upload is explicit opt-in.
- Ambiguity is safer than silent wrong-poster uploads.
