---
name: use-iconify-skill
description: Search Iconify collections with explicit pagination, download SVG icons with attribution metadata, and validate local icon files. Use when selecting, adding, replacing, or checking interface icons; honor the consuming project's collection and style policy.
compatibility: Python 3.9+ with standard library; HTTPS access to api.iconify.design for search and download. Offline checks and tests require no network.
---

# Use Iconify

This skill is portable. Determine the consuming project's allowed collections, styles, destination and theme requirements before selecting icons. Do not impose a particular collection, UI framework, directory layout or programming language.

Resolve `scripts/iconify.py` relative to this skill's directory, not the current working directory. The commands below assume the working directory is the skill directory. In another directory, use the script's absolute path; `--dest` is resolved relative to the caller's working directory.

## Required workflow and boundaries

1. Inspect existing managed icons and reuse a suitable one. Read the project's icon policy. If the project names a collection, all searches and selected IDs must use it.
2. Run the bundled script to search by an English keyword and explicit collection prefix. Read one page, then request another only if needed. Do not invent IDs, call the API through curl or ad hoc code, use a browser to export icons, install framework packages, or download entire collections.
3. Download only the chosen exact `prefix:icon-name` through the script. Pass `--require-current-color` when the project needs theme-colored monochrome icons. Do not manually create, trace or edit the downloaded SVG or its provenance manifest.
4. Run `check` with the project's required prefix, styles and theme constraint. Keep SVG files and `manifest.json` together in source control. Preserve author and license metadata and satisfy the actual license's attribution/distribution requirements; a manifest is a source record, not a replacement for required user-facing attribution.
5. Integrate local SVG through the target platform's native rendering support. Bind `currentColor` to semantic theme colors. Provide accessible names for icon-only actions. Verify the affected integration, without automatically starting a full GUI test suite.

Network, missing-icon, unsupported-SVG and missing-license errors stop acquisition. Report the concrete error; do not bypass validation, silently substitute a different collection, or manufacture a replacement. The script intentionally supports a static SVG subset: some collections use unsupported elements, references or per-icon license differences. Review upstream before expanding support. Never recolor a multicolor illustration by blindly replacing all fills.

## Commands

```sh
# One bounded page. Prefix is always required; any valid collection may be used.
python3 scripts/iconify.py search play --prefix solar --page 1 --page-size 32
python3 scripts/iconify.py search play --prefix solar --page 2 --page-size 32

# Destination is an explicit caller choice; this is only an example.
python3 scripts/iconify.py download solar:play-linear \
  --dest /path/to/icon-directory --require-current-color

# Offline check; repeat --style to allow several suffixes.
python3 scripts/iconify.py check --dest /path/to/icon-directory \
  --prefix solar --style linear --style bold --require-current-color

# Other collections are supported without changing the skill.
python3 scripts/iconify.py search search --prefix mdi --page 1
```

Commands output JSON to stdout. Errors go to stderr and return nonzero; do not treat partial output as success. Run `--help` or `<command> --help` for flags.

## Pagination contract

`--page` is 1-based; `--page-size` defaults to 32 and accepts 32..999. Iconify's `total` is the count returned in its search window, not the total number of matches. The script fetches a bounded window from index zero, increasing its limit with the requested page, then returns only that page. Each page command makes one request, with no background prefetch or automatic traversal.

`next_page` suggests the next explicit request when the window is full; it can lead to an empty final page. `window_count` is not a global result total. Searches stop at the API's 999-result window: `refine_query: true` means use a more specific keyword, not an unbounded pagination loop. Results can change upstream between requests; pages are not a frozen snapshot.

## Storage and validation contract

The destination contains `<prefix>--<icon-name>.svg` and a versioned `manifest.json`. Each entry preserves its exact ID, source/catalog URLs, SHA-256, author and collection license metadata. The script downloads SVG and metadata from the fixed Iconify HTTPS host, refuses redirects, uses a 20-second request timeout and a 2 MiB per-response cap. There are no implicit retries.

Downloads refuse overwrites. Use one writer per destination. The SVG is created exclusively; the manifest is replaced atomically. A failed manifest update rolls back the new SVG during ordinary errors; process interruption can leave an untracked file, which `check` detects. For an intentional upstream update, review and explicitly remove the old managed entry and file before downloading again; never hand-edit SVG bytes to make a checksum pass.

`check` works offline and rejects missing or modified SVG, untracked SVG anywhere under the destination, filename/source mismatches, invalid metadata, linked files, unsupported SVG content, and supplied collection/style/theme policy violations. A missing or empty manifest is an error. It does not scan application source, prove visual correctness, authenticate a manually forged manifest, or sandbox an agent with filesystem/shell access. Project instructions prohibit bypasses; the script enforces the checks on files passed to it.

Without `--require-current-color`, valid static multicolor SVG is preserved. With it, explicit paint values must be `currentColor` or `none`, and the SVG must use `currentColor`; viewBox, path data and opacity are preserved. Rendering support and inherited SVG defaults still require target-platform verification.

## Skill maintenance

Run the offline tests after changing the script:

```sh
python3 -B -m unittest discover -s tests -v
```

Tests use temporary directories and mock network responses. Use `python3 -B` (or `PYTHONDONTWRITEBYTECODE=1`) to avoid leaving bytecode caches beside the skill. Keep one-off scripts, sample downloads and coverage reports in an OS temporary directory, never in the consuming project. Live search/download checks are separate and should use a temporary destination unless a project actually needs the icon. No runtime packages or Node toolchain are required.

API references: [search](https://iconify.design/docs/api/search.html), [SVG](https://iconify.design/docs/api/svg.html), [collection metadata](https://iconify.design/docs/api/collections.html).
