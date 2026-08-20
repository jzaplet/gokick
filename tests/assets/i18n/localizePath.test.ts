import { describe, expect, it } from 'vitest';
import { localizePath } from '@/app-ui/I18n';

// localizePath rebases a stored absolute path (a ?redirect param, a reload
// target) onto the CURRENT language — strips any leading supported-lang
// segment, prefixes the target language's segment ('' for canonical), and
// leaves query + hash untouched.
describe('localizePath', () => {
    it('prefixes the root path for a non-canonical language', () => {
        expect(localizePath('/', 'cs')).toBe('/cs');
    });

    it('strips the prefix when rebasing onto the canonical language', () => {
        expect(localizePath('/cs', 'en')).toBe('/');
        expect(localizePath('/cs/admin/users', 'en')).toBe('/admin/users');
    });

    it('leaves a canonical bare path untouched', () => {
        expect(localizePath('/', 'en')).toBe('/');
        expect(localizePath('/admin/users', 'en')).toBe('/admin/users');
    });

    it('adds the prefix to a bare path and never stacks an existing one', () => {
        expect(localizePath('/admin/users', 'cs')).toBe('/cs/admin/users');
        expect(localizePath('/cs/admin/users', 'cs')).toBe('/cs/admin/users');
    });

    it('preserves query and hash', () => {
        expect(localizePath('/cs/x?y=1#h', 'en')).toBe('/x?y=1#h');
        expect(localizePath('/x?y=1#h', 'cs')).toBe('/cs/x?y=1#h');
        expect(localizePath('/cs?y=1', 'cs')).toBe('/cs?y=1');
    });

    it('only treats the first path segment as a language prefix', () => {
        expect(localizePath('/csx/foo', 'en')).toBe('/csx/foo');
        expect(localizePath('/admin/cs', 'en')).toBe('/admin/cs');
    });
});
