// Frontend runtime config. The Go server injects deployment-specific values
// (Sentry DSN, environment, debug flag) into index.html as <meta> tags at serve
// time, so a single built bundle serves every environment. When a tag is absent
// — e.g. under the Vite dev server, which serves index.html directly — we fall
// back to the build-time import.meta.env values.

const metaContent = (name: string): string | undefined => {
    const el = document.querySelector(`meta[name="gokick:${name}"]`);

    if (el === null) {
        return undefined;
    }
    const content = el.getAttribute('content');

    if (content === null || content === '') {
        return undefined;
    }

    return content;
};

export const sentryDsn = (): string | undefined =>
    metaContent('sentry-dsn') ?? import.meta.env.VITE_SENTRY_DSN;

export const sentryEnvironment = (): string =>
    metaContent('sentry-environment') ?? import.meta.env.VITE_SENTRY_ENVIRONMENT ?? 'development';

export const sentryRelease = (): string | undefined => {
    const release = import.meta.env.VITE_SENTRY_RELEASE;

    if (release === undefined || release === '') {
        return undefined;
    }

    return release;
};

export const sentryDebugEnabled = (): boolean => metaContent('sentry-debug') === 'true';
