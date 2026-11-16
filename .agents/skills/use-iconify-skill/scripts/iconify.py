#!/usr/bin/env python3
"""Bounded Iconify search, SVG acquisition, and offline provenance checks."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import re
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET

BASE = 'https://api.iconify.design'
MAX_BYTES = 2 * 1024 * 1024
NAME = re.compile(r'[a-z0-9]+(?:-[a-z0-9]+)*\Z')


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise urllib.error.HTTPError(req.full_url, code, 'redirect refused', headers, fp)


def request(path):
    if not path.startswith('/') or path.startswith('//'):
        raise ValueError('only relative Iconify API paths are accepted')
    req = urllib.request.Request(BASE + path, headers={'User-Agent': 'use-iconify-skill/1'})
    with urllib.request.build_opener(NoRedirect()).open(req, timeout=20) as response:
        data = response.read(MAX_BYTES + 1)
    if len(data) > MAX_BYTES:
        raise ValueError('API response exceeds 2 MiB')
    return data


def get_json(path):
    data = json.loads(request(path))
    if not isinstance(data, dict):
        raise ValueError('expected an API JSON object')
    return data


def name(value):
    if not isinstance(value, str) or not NAME.fullmatch(value):
        raise ValueError('names must use lowercase letters, digits and single hyphens')
    return value


def split_id(value):
    parts = value.split(':')
    if len(parts) != 2:
        raise ValueError('expected prefix:icon-name')
    return tuple(name(part) for part in parts)


def search(query, prefix, page, page_size):
    name(prefix)
    if not query.strip() or page < 1 or not 32 <= page_size <= 999:
        raise ValueError('query required; page >= 1; page-size must be 32..999')
    start = (page - 1) * page_size
    if start >= 999:
        raise ValueError('search window is limited to 999 icons; refine the query')
    # API limit bounds the entire search window, not a page after start.
    # Fetch a bounded window from zero and slice locally to avoid skipped results.
    window = min(page * page_size, 999)
    params = urllib.parse.urlencode({'query': query, 'prefix': prefix, 'limit': window, 'start': 0})
    data = get_json('/search?' + params)
    found, total = data.get('icons'), data.get('total')
    if (not isinstance(found, list) or type(total) is not int or total != len(found)
            or data.get('limit') != window or len(found) > window):
        raise ValueError('unexpected search response or server page-size change')
    for icon in found:
        if not isinstance(icon, str) or split_id(icon)[0] != prefix:
            raise ValueError('search returned a different collection')
    more = total == window
    return {'icons': found[start:window], 'window_count': total, 'page': page, 'page_size': page_size,
            'start': start, 'next_page': page + 1 if more and window < 999 else None,
            'refine_query': more and window == 999}


def validate_svg(data, monochrome):
    if b'<!' in data:
        raise ValueError('SVG declarations and entities are not supported')
    try:
        root = ET.fromstring(data)
    except ET.ParseError as exc:
        raise ValueError('invalid SVG XML') from exc
    local = lambda tag: tag.rsplit('}', 1)[-1]
    if local(root.tag) != 'svg':
        raise ValueError('expected SVG root')
    try:
        box = [float(v) for v in root.attrib.get('viewBox', '').replace(',', ' ').split()]
        if len(box) != 4 or not all(math.isfinite(v) for v in box) or min(box[2:]) <= 0:
            raise ValueError('invalid SVG viewBox')
    except ValueError as exc:
        raise ValueError('valid SVG viewBox is required') from exc
    allowed = {'svg', 'g', 'path', 'circle', 'ellipse', 'rect', 'line', 'polyline', 'polygon',
               'defs', 'clipPath', 'mask', 'linearGradient', 'radialGradient', 'stop', 'title', 'desc'}
    has_current = False
    for element in root.iter():
        if local(element.tag) not in allowed:
            raise ValueError('unsupported SVG element: ' + local(element.tag))
        for attr, value in element.attrib.items():
            attr = local(attr)
            if attr.lower().startswith('on') or attr in {'href', 'style'}:
                raise ValueError('SVG executable, linked or inline-style content refused')
            for ref in re.findall(r'url\((.*?)\)', value, re.I):
                if not re.fullmatch(r'#[A-Za-z0-9_-]+', ref):
                    raise ValueError('SVG external reference refused')
            if attr in {'fill', 'stroke', 'stop-color', 'color'}:
                has_current |= value == 'currentColor'
                if monochrome and value not in {'none', 'currentColor'}:
                    raise ValueError('theme icon paint must be currentColor or none')
    if monochrome and not has_current:
        raise ValueError('theme icon must use currentColor')


def load_manifest(dest):
    path = dest / 'manifest.json'
    if path.is_symlink():
        raise ValueError('manifest symlinks are not supported')
    if not path.exists():
        return {'version': 1, 'icons': {}}
    data = json.loads(path.read_text(encoding='utf-8'))
    if (not isinstance(data, dict) or data.get('version') != 1
            or not isinstance(data.get('icons'), dict)):
        raise ValueError('invalid manifest schema')
    return data


def save_manifest(dest, manifest):
    with tempfile.NamedTemporaryFile(mode='w', encoding='utf-8', dir=dest, delete=False) as f:
        tmp = Path(f.name)
        try:
            json.dump(manifest, f, ensure_ascii=False, indent=2, sort_keys=True)
            f.write('\n')
            f.close()
            tmp.replace(dest / 'manifest.json')
        finally:
            tmp.unlink(missing_ok=True)


def provenance(info):
    if not isinstance(info, dict):
        raise ValueError('collection metadata is required')
    author, license_info = info.get('author'), info.get('license')
    if (not isinstance(author, dict) or not author.get('name')
            or not isinstance(license_info, dict) or not license_info.get('title')
            or not str(license_info.get('url', '')).startswith('https://')):
        raise ValueError('author and license title/HTTPS URL are required; review upstream metadata')
    return author, license_info


def download(icon, dest, monochrome):
    prefix, icon_name = split_id(icon)
    dest = Path(dest)
    manifest = load_manifest(dest)
    filename = prefix + '--' + icon_name + '.svg'
    target = dest / filename
    if icon in manifest['icons'] or target.exists() or target.is_symlink():
        raise ValueError('icon already exists; no implicit overwrite')
    author, license_info = provenance(get_json('/collections?prefixes=' + prefix).get(prefix))
    route = '/' + prefix + '/' + icon_name + '.svg'
    svg = request(route)  # HTTP 404 is an error: never manufacture a missing icon.
    validate_svg(svg, monochrome)
    entry = {'file': filename, 'source': BASE + route,
             'catalog': 'https://icon-sets.iconify.design/' + prefix + '/' + icon_name + '/',
             'sha256': hashlib.sha256(svg).hexdigest(), 'author': author, 'license': license_info}
    manifest['icons'][icon] = entry
    dest.mkdir(parents=True, exist_ok=True)
    with target.open('xb') as f:
        try:
            f.write(svg)
            f.close()
            save_manifest(dest, manifest)
        except BaseException:
            target.unlink(missing_ok=True)
            raise
    return {'id': icon, **entry}


def check(dest, prefix, styles, monochrome):
    dest = Path(dest)
    if prefix:
        name(prefix)
    for style in styles:
        name(style)
    manifest = load_manifest(dest)
    if not manifest['icons']:
        raise ValueError('no managed icons; missing or empty manifest')
    expected = set()
    for icon, entry in manifest['icons'].items():
        collection, icon_name = split_id(icon)
        filename = collection + '--' + icon_name + '.svg'
        if not isinstance(entry, dict) or entry.get('file') != filename:
            raise ValueError('manifest filename mismatch: ' + icon)
        if prefix and collection != prefix:
            raise ValueError('collection policy mismatch: ' + icon)
        if styles and not any(icon_name.endswith('-' + style) for style in styles):
            raise ValueError('style policy mismatch: ' + icon)
        provenance(entry)
        if entry.get('source') != BASE + '/' + collection + '/' + icon_name + '.svg':
            raise ValueError('source mismatch: ' + icon)
        path = dest / filename
        if path.is_symlink() or not path.is_file():
            raise ValueError('missing SVG or symlink: ' + icon)
        data = path.read_bytes()
        if hashlib.sha256(data).hexdigest() != entry.get('sha256'):
            raise ValueError('SVG checksum mismatch: ' + icon)
        validate_svg(data, monochrome)
        expected.add(filename)
    actual = {str(p.relative_to(dest)) for p in dest.rglob('*') if p.suffix.lower() == '.svg'}
    if actual != expected:
        raise ValueError('untracked SVG files: ' + ', '.join(sorted(actual - expected)))
    return {'valid': True, 'count': len(expected)}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    query = commands.add_parser('search', help='one explicit page from one collection')
    query.add_argument('query')
    query.add_argument('--prefix', required=True)
    query.add_argument('--page', type=int, default=1)
    query.add_argument('--page-size', type=int, default=32)
    fetch = commands.add_parser('download', help='download one exact icon with provenance')
    fetch.add_argument('icon')
    fetch.add_argument('--dest', type=Path, required=True)
    fetch.add_argument('--require-current-color', action='store_true')
    verify = commands.add_parser('check', help='offline manifest, SVG and policy validation')
    verify.add_argument('--dest', type=Path, required=True)
    verify.add_argument('--prefix')
    verify.add_argument('--style', action='append', default=[])
    verify.add_argument('--require-current-color', action='store_true')
    args = parser.parse_args(argv)
    try:
        if args.command == 'search':
            result = search(args.query, args.prefix, args.page, args.page_size)
        elif args.command == 'download':
            result = download(args.icon, args.dest, args.require_current_color)
        else:
            result = check(args.dest, args.prefix, args.style, args.require_current_color)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0
    except (ValueError, OSError, urllib.error.URLError) as exc:
        print('error: ' + str(exc), file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
