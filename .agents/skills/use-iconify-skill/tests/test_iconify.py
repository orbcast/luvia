"""Offline contract tests; no network or external packages required."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).parents[1] / 'scripts' / 'iconify.py'
spec = importlib.util.spec_from_file_location('iconify', SCRIPT)
icons = importlib.util.module_from_spec(spec)
if SCRIPT.exists():
    spec.loader.exec_module(icons)

SVG = b'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M0 0h24v24z"/></svg>'
INFO = {'name': 'Example', 'author': {'name': 'Artist'}, 'license': {'title': 'CC BY 4.0', 'url': 'https://creativecommons.org/licenses/by/4.0/'}}


class SearchTests(unittest.TestCase):
    def test_pagination_uses_bounded_start_and_prefix(self):
        found = ['solar:icon-' + str(i) for i in range(64)]
        with patch.object(icons, 'get_json', return_value={'icons': found, 'total': 64, 'limit': 64}) as get:
            result = icons.search('play pause', 'solar', 2, 32)
        self.assertEqual(result['start'], 32)
        self.assertEqual(result['next_page'], 3)
        self.assertEqual(result['icons'], found[32:])
        self.assertEqual(get.call_args.args[0], '/search?query=play+pause&prefix=solar&limit=64&start=0')

    def test_search_cap_requires_refined_query(self):
        found = ['solar:icon-' + str(i) for i in range(999)]
        with patch.object(icons, 'get_json', return_value={'icons': found, 'total': 999, 'limit': 999}):
            result = icons.search('play', 'solar', 32, 32)
        self.assertEqual(len(result['icons']), 7)
        self.assertTrue(result['refine_query'])
        self.assertIsNone(result['next_page'])
        with self.assertRaises(ValueError):
            icons.search('play', 'solar', 33, 32)

    def test_first_page_at_capacity_suggests_second(self):
        found = ['solar:icon-' + str(i) for i in range(32)]
        with patch.object(icons, 'get_json', return_value={'icons': found, 'total': 32, 'limit': 32}):
            result = icons.search('play', 'solar', 1, 32)
        self.assertEqual(result['next_page'], 2)
        self.assertEqual(result['window_count'], 32)
        self.assertFalse(result['refine_query'])

    def test_second_page_beyond_last_is_empty(self):
        with patch.object(icons, 'get_json', return_value={'icons': ['solar:play-linear'], 'total': 1, 'limit': 64}):
            result = icons.search('play', 'solar', 2, 32)
        self.assertEqual(result['icons'], [])
        self.assertIsNone(result['next_page'])

    def test_last_page_and_empty_results(self):
        for total, found in [(1, ['solar:play-linear']), (0, [])]:
            with self.subTest(total=total), patch.object(icons, 'get_json', return_value={'icons': found, 'total': total, 'limit': 32}):
                self.assertIsNone(icons.search('play', 'solar', 1, 32)['next_page'])

    def test_reject_invalid_page_query_and_prefix(self):
        for args in [('play', 'solar', 0, 32), ('play', 'solar', 1, 31), ('play', 'solar', 1, 1000), (' ', 'solar', 1, 32), ('play', '../solar', 1, 32)]:
            with self.subTest(args=args), self.assertRaises(ValueError):
                icons.search(*args)

    def test_reject_malformed_or_cross_collection_results(self):
        for data in [{}, {'icons': [], 'total': -1}, {'icons': ['other:play'], 'total': 1, 'limit': 32}, {'icons': [], 'total': 99, 'limit': 64}]:
            with self.subTest(data=data), patch.object(icons, 'get_json', return_value=data), self.assertRaises(ValueError):
                icons.search('play', 'solar', 1, 32)


class SvgTests(unittest.TestCase):
    def test_valid_monochrome_and_duotone(self):
        icons.validate_svg(SVG, True)
        icons.validate_svg(SVG.replace(b'd="', b'opacity="0.5" d="'), True)

    def test_multicolor_is_opt_in_by_omitting_monochrome_requirement(self):
        colorful = SVG.replace(b'currentColor', b'#ff0000')
        icons.validate_svg(colorful, False)
        with self.assertRaises(ValueError):
            icons.validate_svg(colorful, True)

    def test_reject_active_content_with_valid_viewbox(self):
        # Keep the rest of the SVG valid so each sample reaches its own guard.
        samples = [
            SVG.replace(b'<path ', b'<script '),
            SVG.replace(b'<path ', b'<path onload="alert(1)" '),
            SVG.replace(b'<path ', b'<path href="https://example.com/a" '),
            SVG.replace(b'<path ', b'<path style="fill:red" '),
            SVG.replace(b'currentColor', b'url(https://example.com/a)'),
            SVG.replace(b'currentColor', b'none'),
        ]
        for sample in samples:
            with self.subTest(sample=sample), self.assertRaises(ValueError):
                icons.validate_svg(sample, True)
        # Internal references are accepted for general SVG, but not theme paint.
        internal = SVG.replace(b'currentColor', b'url(#gradient)')
        icons.validate_svg(internal, False)
        with self.assertRaises(ValueError):
            icons.validate_svg(internal, True)

    def test_reject_unsafe_or_invalid_svg(self):
        samples = [b'not svg', b'<html/>', b'<!DOCTYPE svg><svg/>', b'<svg><script/></svg>', b'<svg onload="x"/>', b'<svg><image href="https://example.com/a"/></svg>', b'<svg><path fill="url(https://example.com/a)"/></svg>', b'<svg><style>path{fill:red}</style></svg>', b'<svg><path style="fill:red"/></svg>', b'<svg viewBox="0 0 0 24"/>', b'<svg viewBox="0 0 nan 24"/>']
        for sample in samples:
            with self.subTest(sample=sample), self.assertRaises(ValueError):
                icons.validate_svg(sample, True)


class StoreTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dest = Path(self.tmp.name) / 'icons'

    def download(self, icon='solar:play-linear', svg=SVG, info=INFO):
        with patch.object(icons, 'get_json', return_value={icon.split(':')[0]: info}) as metadata, patch.object(icons, 'request', return_value=svg):
            result = icons.download(icon, self.dest, True)
            metadata.assert_called_once_with('/collections?prefixes=' + icon.split(':')[0])
            return result

    def test_download_records_source_license_and_checksum(self):
        result = self.download()
        self.assertEqual((self.dest / 'solar--play-linear.svg').read_bytes(), SVG)
        manifest = json.loads((self.dest / 'manifest.json').read_text())
        entry = manifest['icons']['solar:play-linear']
        self.assertEqual(entry['license'], INFO['license'])
        self.assertEqual(entry['author'], INFO['author'])
        self.assertEqual(entry['source'], 'https://api.iconify.design/solar/play-linear.svg')
        self.assertEqual(len(entry['sha256']), 64)
        self.assertEqual(result['id'], 'solar:play-linear')
        self.assertEqual(icons.check(self.dest, 'solar', ['linear', 'bold'], True)['count'], 1)

    def test_other_collections_work(self):
        self.download('mdi:play')
        self.assertEqual(icons.check(self.dest, None, [], True)['count'], 1)
        with self.assertRaises(ValueError):
            icons.check(self.dest, 'solar', [], True)

    def test_no_overwrite(self):
        self.download()
        original = (self.dest / 'manifest.json').read_bytes()
        with self.assertRaises(ValueError):
            self.download()
        self.assertEqual((self.dest / 'manifest.json').read_bytes(), original)

    def test_failed_manifest_write_rolls_back_svg(self):
        with patch.object(icons, 'save_manifest', side_effect=OSError('disk full')), self.assertRaises(OSError):
            self.download()
        self.assertFalse((self.dest / 'solar--play-linear.svg').exists())

    def test_invalid_id_license_or_svg_writes_nothing(self):
        for icon, svg, info in [('../solar:play', SVG, INFO), ('solar:../play', SVG, INFO), ('solar:Play', SVG, INFO), ('solar:play', b'bad', INFO), ('solar:play', SVG, {}), ('solar:play', SVG, {'license': {'title': 'x'}})]:
            with self.subTest(icon=icon, info=info), self.assertRaises(ValueError):
                self.download(icon, svg, info)
            self.assertFalse((self.dest / 'manifest.json').exists())

    def test_hash_missing_file_untracked_and_style_checks(self):
        self.download()
        path = self.dest / 'solar--play-linear.svg'
        path.write_bytes(SVG + b' ')
        with self.assertRaises(ValueError):
            icons.check(self.dest, 'solar', [], True)
        path.unlink()
        with self.assertRaises(ValueError):
            icons.check(self.dest, 'solar', [], True)
        path.write_bytes(SVG)
        with self.assertRaises(ValueError):
            icons.check(self.dest, 'solar', ['bold'], True)
        extra = self.dest / 'extra.svg'
        extra.write_bytes(SVG)
        with self.assertRaises(ValueError):
            icons.check(self.dest, 'solar', [], True)

    def test_manifest_tampering_and_empty_store_fail(self):
        with self.assertRaises(ValueError):
            icons.check(self.dest, None, [], False)
        self.dest.mkdir()
        for value in ['not json', '{}', '{"version":1,"icons":{}}', '{"version":2,"icons":{}}']:
            (self.dest / 'manifest.json').write_text(value)
            with self.subTest(value=value), self.assertRaises(ValueError):
                icons.check(self.dest, None, [], False)

    def test_manifest_source_license_and_schema_are_validated(self):
        self.download()
        path = self.dest / 'manifest.json'
        original = path.read_text()
        for field, value in [('source', 'https://example.com/icon.svg'), ('license', {}), ('author', {}), ('sha256', 'wrong')]:
            data = json.loads(original)
            data['icons']['solar:play-linear'][field] = value
            path.write_text(json.dumps(data))
            with self.subTest(field=field), self.assertRaises(ValueError):
                icons.check(self.dest, None, [], False)
        path.write_text(original)
        for value in ['[]', 'null', '{"version":1,"icons":[]}']:
            path.write_text(value)
            with self.subTest(value=value), self.assertRaises(ValueError):
                icons.load_manifest(self.dest)

    def test_svg_and_manifest_symlinks_are_rejected(self):
        self.download()
        svg = self.dest / 'solar--play-linear.svg'
        actual = self.dest / 'saved'
        svg.rename(actual)
        svg.symlink_to(actual)
        with self.assertRaises(ValueError):
            icons.check(self.dest, None, [], False)
        manifest = self.dest / 'manifest.json'
        saved = self.dest / 'saved-manifest'
        manifest.rename(saved)
        manifest.symlink_to(saved)
        with self.assertRaises(ValueError):
            icons.load_manifest(self.dest)

    def test_network_error_does_not_create_store(self):
        with patch.object(icons, 'get_json', side_effect=icons.urllib.error.URLError('offline')), self.assertRaises(icons.urllib.error.URLError):
            icons.download('solar:play-linear', self.dest, True)
        self.assertFalse(self.dest.exists())

    def test_check_current_color_is_enforced_offline(self):
        with patch.object(icons, 'get_json', return_value={'solar': INFO}), patch.object(icons, 'request', return_value=SVG.replace(b'currentColor', b'#fff')):
            icons.download('solar:play-linear', self.dest, False)
        self.assertEqual(icons.check(self.dest, 'solar', [], False)['count'], 1)
        with self.assertRaises(ValueError):
            icons.check(self.dest, 'solar', [], True)

    def test_manifest_cannot_redirect_to_arbitrary_file(self):
        self.download()
        path = self.dest / 'manifest.json'
        manifest = json.loads(path.read_text())
        manifest['icons']['solar:play-linear']['file'] = '../elsewhere.svg'
        path.write_text(json.dumps(manifest))
        with self.assertRaises(ValueError):
            icons.check(self.dest, None, [], False)


class HttpAndCliTests(unittest.TestCase):
    def test_request_is_fixed_host_bounded_and_no_redirect(self):
        response = unittest.mock.MagicMock()
        response.__enter__.return_value = response
        response.status = 200
        response.read.return_value = SVG
        opener = unittest.mock.Mock()
        opener.open.return_value = response
        with patch.object(icons.urllib.request, 'build_opener', return_value=opener):
            self.assertEqual(icons.request('/solar/play-linear.svg'), SVG)
        req = opener.open.call_args.args[0]
        self.assertEqual(req.full_url, 'https://api.iconify.design/solar/play-linear.svg')
        self.assertEqual(opener.open.call_args.kwargs['timeout'], 20)
        with self.assertRaises(ValueError):
            icons.request('https://example.com')
        response.read.return_value = b'x' * (icons.MAX_BYTES + 1)
        with patch.object(icons.urllib.request, 'build_opener', return_value=opener), self.assertRaises(ValueError):
            icons.request('/solar/play-linear.svg')
        with self.assertRaises(icons.urllib.error.HTTPError):
            icons.NoRedirect().redirect_request(req, None, 302, 'redirect', {}, 'https://example.com')

    def test_json_error_and_shape(self):
        for content in [b'bad', b'[]']:
            with patch.object(icons, 'request', return_value=content), self.assertRaises(ValueError):
                icons.get_json('/search')

    def test_cli_download_and_check_forward_policy_flags(self):
        with patch.object(icons, 'download', return_value={'id': 'mdi:play'}) as fetch, contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(icons.main(['download', 'mdi:play', '--dest', 'icons', '--require-current-color']), 0)
            fetch.assert_called_once_with('mdi:play', Path('icons'), True)
        with patch.object(icons, 'check', return_value={'valid': True}) as verify, contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(icons.main(['check', '--dest', 'icons', '--prefix', 'solar', '--style', 'linear', '--style', 'bold', '--require-current-color']), 0)
            verify.assert_called_once_with(Path('icons'), 'solar', ['linear', 'bold'], True)
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit) as err:
            icons.main(['search', 'play'])
        self.assertEqual(err.exception.code, 2)

    def test_http_failure_is_not_retried(self):
        opener = unittest.mock.Mock()
        opener.open.side_effect = icons.urllib.error.HTTPError('url', 404, 'not found', {}, None)
        with patch.object(icons.urllib.request, 'build_opener', return_value=opener), self.assertRaises(icons.urllib.error.HTTPError):
            icons.request('/solar/missing.svg')
        self.assertEqual(opener.open.call_count, 1)

    def test_cli_success_and_failure(self):
        with patch.object(icons, 'search', return_value={'icons': []}), contextlib.redirect_stdout(io.StringIO()) as out:
            self.assertEqual(icons.main(['search', 'play', '--prefix', 'solar']), 0)
            self.assertEqual(json.loads(out.getvalue()), {'icons': []})
        with contextlib.redirect_stderr(io.StringIO()) as err:
            self.assertEqual(icons.main(['download', '../bad', '--dest', '/unused']), 1)
            self.assertIn('error:', err.getvalue())


if __name__ == '__main__':
    unittest.main()
