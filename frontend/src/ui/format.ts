// Formatting helpers ported from the Tether prototype's lib/format.ts.
// Kept separate from the root src/format.ts, which the old (disconnected)
// screens still use.

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

export function formatBytes(bytes: number, digits?: number): string {
    if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
    const i = Math.min(UNITS.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
    const value = bytes / Math.pow(1024, i);
    const d = digits ?? (i === 0 ? 0 : value >= 100 ? 0 : value >= 10 ? 1 : 2);
    return `${value.toFixed(d)} ${UNITS[i]}`;
}

export function formatSpeed(bps: number): string {
    if (!Number.isFinite(bps) || bps <= 0) return '—';
    return `${formatBytes(bps)}/s`;
}

export function formatDuration(ms: number): string {
    if (!Number.isFinite(ms) || ms <= 0) return '—';
    const s = Math.round(ms / 1000);
    if (s < 60) return `${s}s`;
    const m = Math.floor(s / 60);
    if (m < 60) return `${m}m ${s % 60}s`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ${m % 60}m`;
    const d = Math.floor(h / 24);
    return `${d}d ${h % 24}h`;
}

export function formatPercent(v: number): string {
    if (!Number.isFinite(v)) return '0%';
    if (v > 0 && v < 0.1) return '0.1%';
    return `${v >= 99.95 ? 100 : v.toFixed(1)}%`;
}

export function relativeTime(iso: string, now = Date.now()): string {
    const diff = now - new Date(iso).getTime();
    if (diff < 5_000) return 'just now';
    if (diff < 60_000) return `${Math.floor(diff / 1000)}s ago`;
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
    if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
    return `${Math.floor(diff / 86_400_000)}d ago`;
}

export function clockTime(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}
