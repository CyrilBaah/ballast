// formatBytes renders a byte count as a human-scale GB figure, shared by
// the sidebar's storage-usage readout and the history screen's byte progress.
export function formatBytes(bytes: number): string {
    const gb = bytes / 1024 ** 3;
    if (gb >= 0.1) return `${gb.toFixed(1)} GB`;
    const mb = bytes / 1024 ** 2;
    if (mb >= 0.1) return `${mb.toFixed(1)} MB`;
    return `${bytes} bytes`;
}
