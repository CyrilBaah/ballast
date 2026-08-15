// Icon set ported from the Tether prototype's primitives.tsx `Icon` object.
// Each function returns an inline SVG string; `cls` defaults to "icon".

const BASE = 'icon';

export const icon = {
    bolt: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M13 2 4.5 13.5H11l-1 8.5 8.5-11.5H12l1-8.5Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    home: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M4 10.5 12 4l8 6.5V19a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 4 19v-8.5Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="M9.5 20.5v-6h5v6" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    layers: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="m12 3 8.5 4.5L12 12 3.5 7.5 12 3Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="m4 12.5 8 4.2 8-4.2M4 16.8l8 4.2 8-4.2" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    pulse: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M2 12h4l2.5-7 4 14L15.5 12H22" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    flask: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M9 3h6M10 3v6.2L4.8 18a2 2 0 0 0 1.7 3h11a2 2 0 0 0 1.7-3L14 9.2V3" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="M7.2 15h9.6" stroke="currentColor" stroke-width="1.6"/></svg>`,
    gear: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><circle cx="12" cy="12" r="3.2" stroke="currentColor" stroke-width="1.6"/><path d="M12 2.6v2.2M12 19.2v2.2M21.4 12h-2.2M4.8 12H2.6m15.1-6.6-1.6 1.6M7.9 16.1l-1.6 1.6m11.4 0-1.6-1.6M7.9 7.9 6.3 6.3" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>`,
    play: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M8 5.5 18 12 8 18.5v-13Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    pause: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M9 5v14M15 5v14" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    x: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="m6 6 12 12M18 6 6 18" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>`,
    plus: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    shield: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M12 2.8 20 6v6c0 4.6-3.3 7.9-8 9.2C7.3 19.9 4 16.6 4 12V6l8-3.2Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="m8.8 12.2 2.2 2.2 4.2-4.4" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    refresh: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M20 12a8 8 0 1 1-2.6-5.9M20 4v4.5h-4.5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    alert: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M12 4.5 21 20H3l9-15.5Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="M12 10v4.2M12 17h.01" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>`,
    check: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="m5 12.5 4.5 4.5L19 7" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    chip: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><rect x="6.5" y="6.5" width="11" height="11" rx="2.5" stroke="currentColor" stroke-width="1.6"/><path d="M10 3v3.2M14 3v3.2M10 17.8V21M14 17.8V21M3 10h3.2M3 14h3.2M17.8 10H21M17.8 14H21" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>`,
    film: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><rect x="3" y="5" width="18" height="14" rx="3" stroke="currentColor" stroke-width="1.6"/><path d="M8 5v14M16 5v14M3 12h18" stroke="currentColor" stroke-width="1.4"/></svg>`,
    wave: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M4 10v4M8 6.5v11M12 3.5v17M16 7.5v9M20 10v4" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    box: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M12 3 4 7v10l8 4 8-4V7l-8-4Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="m4 7 8 4 8-4M12 11v10" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    image: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><rect x="3" y="4.5" width="18" height="15" rx="3" stroke="currentColor" stroke-width="1.6"/><circle cx="9" cy="10" r="1.6" stroke="currentColor" stroke-width="1.5"/><path d="m4 17 5-4.5 4.5 4 3-2.5L20 18" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    doc: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M6 3.5h7L18.5 9v11.5h-12V3.5Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="M13 3.5V9h5.5" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    folder: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M3.5 7.5A2 2 0 0 1 5.5 5.5H10l2 2.4h6.5a2 2 0 0 1 2 2v7.6a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2V7.5Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    drive: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M8.6 3.5h6.8l6 11.6h-6.8L8.6 3.5Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M8.6 3.5 2.6 15.1l3.4 5.4 6-11.6L8.6 3.5Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M6 20.5h12l3.4-5.4H9.4L6 20.5Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/></svg>`,
    card: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M6 3.5h8.5L19 8v12.5H6V3.5Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/><path d="M9.5 6.5v3M12.5 6.5v3M15.5 8v1.5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>`,
    disk: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><rect x="3" y="6" width="18" height="12" rx="3" stroke="currentColor" stroke-width="1.6"/><circle cx="7.5" cy="12" r="1.2" fill="currentColor"/><path d="M11 12h7" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>`,
    laptop: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><rect x="4" y="5" width="16" height="11" rx="2.5" stroke="currentColor" stroke-width="1.6"/><path d="M2 19h20" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>`,
    arrow: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M4.5 12h15m0 0-6-6m6 6-6 6" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    chevron: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="m9 5 7 7-7 7" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    heart: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M12 20s-7.5-4.3-7.5-9.4A4.1 4.1 0 0 1 12 8.3a4.1 4.1 0 0 1 7.5 2.3C19.5 15.7 12 20 12 20Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
    sparkle: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" fill="none" class="${cls}"><path d="M12 3.5c.7 4.2 1.8 5.3 6 6-4.2.7-5.3 1.8-6 6-.7-4.2-1.8-5.3-6-6 4.2-.7 5.3-1.8 6-6Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M19 15.5c.3 1.7.7 2.2 2.5 2.5-1.8.3-2.2.8-2.5 2.5-.3-1.7-.7-2.2-2.5-2.5 1.8-.3 2.2-.8 2.5-2.5Z" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>`,
    google: (cls = BASE) =>
        `<svg viewBox="0 0 24 24" class="${cls}"><path fill="#4285F4" d="M21.6 12.23c0-.68-.06-1.34-.18-1.96H12v3.72h5.38a4.6 4.6 0 0 1-2 3.02v2.5h3.24c1.9-1.74 2.98-4.3 2.98-7.28Z"/><path fill="#34A853" d="M12 22c2.7 0 4.96-.9 6.62-2.43l-3.24-2.51c-.9.6-2.05.96-3.38.96-2.6 0-4.8-1.76-5.59-4.12H3.06v2.59A10 10 0 0 0 12 22Z"/><path fill="#FBBC05" d="M6.41 13.9a6 6 0 0 1 0-3.8V7.5H3.06a10 10 0 0 0 0 9l3.35-2.6Z"/><path fill="#EA4335" d="M12 5.98c1.47 0 2.79.5 3.83 1.5l2.87-2.87C16.95 2.99 14.7 2 12 2a10 10 0 0 0-8.94 5.5l3.35 2.6C7.2 7.74 9.4 5.98 12 5.98Z"/></svg>`,
};

export function kindIcon(kind: string, cls = BASE): string {
    if (kind === 'video') return icon.film(cls);
    if (kind === 'audio') return icon.wave(cls);
    if (kind === 'dataset') return icon.chip(cls);
    if (kind === 'image') return icon.image(cls);
    if (kind === 'doc') return icon.doc(cls);
    return icon.box(cls);
}

export const KIND_TONE: Record<string, string> = {
    video: 'tone-grape',
    audio: 'tone-sky',
    dataset: 'tone-grass',
    archive: 'tone-tangerine',
    image: 'tone-coral',
    doc: 'tone-butter',
};
