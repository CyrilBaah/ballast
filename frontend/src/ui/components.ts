// Reusable markup builders — ported from the Tether prototype's primitives.tsx.
// Everything here returns an HTML string; callers insert it via innerHTML and
// wire up any interactivity afterwards (matching this repo's existing
// template-string + querySelector convention).

export type UploadStatus =
    | 'queued'
    | 'uploading'
    | 'reconnecting'
    | 'paused'
    | 'verifying'
    | 'awaiting_confirmation'
    | 'completed'
    | 'failed'
    | 'canceled';

export const STATUS: Record<UploadStatus, { label: string; warm: string }> = {
    uploading: { label: 'Uploading', warm: 'Flowing nicely' },
    reconnecting: { label: 'Reconnecting', warm: 'Holding your place' },
    queued: { label: 'Queued', warm: 'Waiting its turn' },
    paused: { label: 'Paused', warm: 'Parked safely' },
    verifying: { label: 'Verifying', warm: 'Double-checking' },
    awaiting_confirmation: { label: 'Needs you', warm: 'Send it or cancel' },
    completed: { label: 'Safe on Drive', warm: 'Done and verified' },
    failed: { label: 'Failed', warm: 'Could not finish' },
    canceled: { label: 'Canceled', warm: 'Stopped by you' },
};

const LIVE_STATUSES = new Set<UploadStatus>(['uploading', 'reconnecting', 'verifying']);

export function button(
    inner: string,
    opts: {
        variant?: 'primary' | 'ghost' | 'outline' | 'danger' | 'soft';
        size?: 'sm' | 'md' | 'lg' | 'icon';
        className?: string;
        attrs?: string;
    } = {},
): string {
    const variant = opts.variant ?? 'ghost';
    const size = opts.size ?? 'md';
    const sizeClass = size === 'md' ? '' : `btn-${size}`;
    return `<button type="button" class="btn btn-${variant} ${sizeClass} ${opts.className ?? ''}" ${opts.attrs ?? ''}>${inner}</button>`;
}

export function statusChip(status: UploadStatus, small = false): string {
    const s = STATUS[status];
    const live = LIVE_STATUSES.has(status);
    return `<span class="status-chip status-${status} ${small ? 'small' : ''}">
        <span class="status-dot ${live ? 'pulse' : ''}"></span>${s.label}
    </span>`;
}

export function progressBar(
    confirmed: number,
    inflight: number,
    opts: { height?: number; animate?: boolean; tone?: 'grass' | 'tangerine' | 'grape' } = {},
): string {
    const height = opts.height ?? 8;
    const tone = opts.tone ?? 'grass';
    const confirmedPct = Math.min(100, confirmed * 100);
    const inflightWidth = Math.max(0, Math.min(100 - confirmedPct, inflight * 100));
    return `<div class="progress-bar" style="height:${height}px">
        <div class="progress-bar-fill progress-${tone}" style="width:${confirmedPct}%"></div>
        <div class="progress-bar-inflight ${opts.animate ? 'shimmer' : ''}" style="left:${confirmedPct}%;width:${inflightWidth}%"></div>
    </div>`;
}

const CHUNK_LABEL: Record<string, string> = {
    c: 'confirmed by Drive',
    i: 'in flight',
    f: 'failed',
    p: 'not sent yet',
};

export function chunkStrip(map: string): string {
    const cells = map
        .split('')
        .map((c) => `<span class="chunkcell cell-${c === 'c' || c === 'i' || c === 'f' ? c : 'p'} ${c === 'i' ? 'pulse' : ''}"></span>`)
        .join('');
    return `<div class="chunk-strip">${cells}</div>`;
}

export function chunkGrid(map: string): string {
    const cells = map
        .split('')
        .map((c, i) => {
            const cell = c === 'c' || c === 'i' || c === 'f' ? c : 'p';
            return `<span class="chunkcell cell-${cell} ${cell === 'i' ? 'pulse' : ''}" title="Chunk ${i + 1} — ${CHUNK_LABEL[cell]}"></span>`;
        })
        .join('');
    return `<div class="chunk-grid">${cells}</div>`;
}

export function sparkline(data: number[], opts: { color?: string; height?: number; fill?: boolean } = {}): string {
    const color = opts.color ?? '#12a970';
    const height = opts.height ?? 34;
    const fill = opts.fill ?? true;
    const pts = data.length > 1 ? data.slice(-48) : [0, 0];
    const max = Math.max(...pts, 1);
    const w = 100;
    const step = w / (pts.length - 1);
    const coords = pts.map((v, i) => [i * step, height - (v / max) * (height - 4) - 2]);
    const line = coords.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(2)},${y.toFixed(2)}`).join(' ');
    const area = `${line} L${w},${height} L0,${height} Z`;
    const id = `sp-${color.replace('#', '')}-${Math.random().toString(36).slice(2, 8)}`;
    return `<svg viewBox="0 0 ${w} ${height}" preserveAspectRatio="none" class="sparkline">
        <defs><linearGradient id="${id}" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="${color}" stop-opacity="0.28"/>
            <stop offset="100%" stop-color="${color}" stop-opacity="0"/>
        </linearGradient></defs>
        ${fill ? `<path d="${area}" fill="url(#${id})"/>` : ''}
        <path d="${line}" fill="none" stroke="${color}" stroke-width="1.8" vector-effect="non-scaling-stroke" stroke-linejoin="round" stroke-linecap="round"/>
    </svg>`;
}

export function ring(
    value: number,
    opts: { size?: number; stroke?: number; color?: string; track?: string; inner?: string } = {},
): string {
    const size = opts.size ?? 46;
    const stroke = opts.stroke ?? 5;
    const color = opts.color ?? '#12a970';
    const track = opts.track ?? '#efe4d6';
    const r = (size - stroke) / 2;
    const c = 2 * Math.PI * r;
    const offset = c - c * Math.min(1, Math.max(0, value));
    return `<div class="ring-wrap" style="width:${size}px;height:${size}px">
        <svg width="${size}" height="${size}">
            <circle cx="${size / 2}" cy="${size / 2}" r="${r}" fill="none" stroke="${track}" stroke-width="${stroke}"/>
            <circle cx="${size / 2}" cy="${size / 2}" r="${r}" fill="none" stroke="${color}" stroke-width="${stroke}" stroke-linecap="round" stroke-dasharray="${c}" stroke-dashoffset="${offset}"/>
        </svg>
        <div class="ring-content">${opts.inner ?? ''}</div>
    </div>`;
}

export function toggleHtml(checked: boolean, attrs = ''): string {
    return `<button type="button" class="toggle ${checked ? 'on' : ''}" role="switch" aria-checked="${checked}" ${attrs}>
        <span class="toggle-thumb"></span>
    </button>`;
}

const AVATAR_GRADIENTS = 5;

export function avatarSeedIndex(seed: string): number {
    return Math.abs(seed.split('').reduce((a, ch) => a + ch.charCodeAt(0), 0)) % AVATAR_GRADIENTS;
}

export function avatar(seed: string, name: string, size = 34): string {
    const idx = avatarSeedIndex(seed);
    const initials =
        name
            .split(/\s+/)
            .slice(0, 2)
            .map((w) => w[0]?.toUpperCase() ?? '')
            .join('') || 'U';
    return `<span class="avatar avatar-grad-${idx}" style="width:${size}px;height:${size}px;font-size:${size * 0.36}px">${initials}</span>`;
}

export { icon, kindIcon, KIND_TONE } from './icons';
