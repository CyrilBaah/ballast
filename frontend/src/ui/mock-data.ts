// Static in-memory sample data for the Ballast preview UI. Nothing here
// calls the real Wails/Drive APIs (see src/api/*.ts for those) — this is
// purely a realistic-looking snapshot for the frontend to render, ported
// and re-themed from the Tether prototype's lib/types.ts sample state.

import type { UploadStatus } from './components';

export type NetworkQuality = 'fiber' | 'good' | 'poor' | 'edge' | 'offline';

export type UploadDTO = {
    id: string;
    name: string;
    kind: string;
    sizeBytes: number;
    chunkSizeBytes: number;
    totalChunks: number;
    confirmedChunks: number;
    bytesConfirmed: number;
    inflightBytes: number;
    bytesDiscarded: number;
    bytesProtected: number;
    status: UploadStatus;
    destination: string;
    sessionUri: string | null;
    driveFileId: string | null;
    checksum: string;
    resumeCount: number;
    crashRecoveries: number;
    throughputBps: number;
    throughputHistory: number[];
    retryRemainingMs: number;
    elapsedMs: number;
    lastError: string | null;
    note: string | null;
    createdAt: string;
    /** one char per chunk: p pending, i inflight, c confirmed, f failed */
    chunkMap: string;
};

export type EventDTO = {
    id: string;
    uploadId: string | null;
    level: 'info' | 'warn' | 'error' | 'success';
    message: string;
    detail: string | null;
    createdAt: string;
};

export type SettingsDTO = {
    networkQuality: NetworkQuality;
    autoResume: boolean;
    verifyChecksums: boolean;
    maxConcurrent: number;
    bandwidthCapMbps: number;
    timeScale: number;
    sessionsRecovered: number;
    netHistory: number[];
    userName: string;
    userEmail: string;
    avatarSeed: string;
    driveFolder: string;
    storageUsedBytes: number;
    storageTotalBytes: number;
};

function chunkMapFor(total: number, confirmed: number, extra: 'i' | 'f' | null = null): string {
    let map = 'c'.repeat(confirmed);
    if (extra && confirmed < total) map += extra;
    map += 'p'.repeat(Math.max(0, total - map.length));
    return map.slice(0, total);
}

function hoursAgo(h: number): string {
    return new Date(Date.now() - h * 3_600_000).toISOString();
}
function minsAgo(m: number): string {
    return new Date(Date.now() - m * 60_000).toISOString();
}

const GB = 1024 ** 3;
const MB = 1024 ** 2;

export const uploads: UploadDTO[] = [
    {
        id: 'u1',
        name: 'A001_C014_0417RB.mov',
        kind: 'video',
        sizeBytes: 96 * GB,
        chunkSizeBytes: 64 * MB,
        totalChunks: 1536,
        confirmedChunks: 980,
        bytesConfirmed: 61.3 * GB,
        inflightBytes: 128 * MB,
        bytesDiscarded: 64 * MB,
        bytesProtected: 61.3 * GB,
        status: 'uploading',
        destination: 'Field Uploads / Kyiv',
        sessionUri: 'https://drive.googleapis.com/upload/resumable/session/AB12xyz',
        driveFileId: null,
        checksum: '9f2a7c1e4b6d0a3f8e5c2b1d7a4f6e9c',
        resumeCount: 3,
        crashRecoveries: 1,
        throughputBps: 9.4 * MB,
        throughputHistory: [2, 4, 6, 5, 7, 8, 6, 9, 10, 9, 8, 9, 11, 10, 9],
        retryRemainingMs: 0,
        elapsedMs: 41 * 60_000,
        lastError: null,
        note: 'A-cam, day 4',
        createdAt: hoursAgo(1.2),
        chunkMap: chunkMapFor(64, 41, 'i'),
    },
    {
        id: 'u2',
        name: 'int_015_bohdan_multitrack.wav',
        kind: 'audio',
        sizeBytes: 33 * GB,
        chunkSizeBytes: 32 * MB,
        totalChunks: 1056,
        confirmedChunks: 640,
        bytesConfirmed: 20 * GB,
        inflightBytes: 0,
        bytesDiscarded: 32 * MB,
        bytesProtected: 20 * GB,
        status: 'reconnecting',
        destination: 'Field Uploads / Kyiv',
        sessionUri: 'https://drive.googleapis.com/upload/resumable/session/CD34abc',
        driveFileId: null,
        checksum: '3b8d5f2a9c1e7b4d6a0f3c8e5b2d9a4f',
        resumeCount: 6,
        crashRecoveries: 0,
        throughputBps: 0,
        throughputHistory: [5, 6, 4, 3, 2, 1, 0, 0, 1, 0],
        retryRemainingMs: 4200,
        elapsedMs: 58 * 60_000,
        lastError: 'Connection reset while sending chunk 641',
        note: 'Interview, day 5',
        createdAt: hoursAgo(1.6),
        chunkMap: chunkMapFor(64, 39, null),
    },
    {
        id: 'u3',
        name: 'sensor-array-2026-Q1.parquet.tar',
        kind: 'dataset',
        sizeBytes: 240 * GB,
        chunkSizeBytes: 128 * MB,
        totalChunks: 1920,
        confirmedChunks: 0,
        bytesConfirmed: 0,
        inflightBytes: 0,
        bytesDiscarded: 0,
        bytesProtected: 0,
        status: 'queued',
        destination: 'Research / Cold Storage',
        sessionUri: null,
        driveFileId: null,
        checksum: '—',
        resumeCount: 0,
        crashRecoveries: 0,
        throughputBps: 0,
        throughputHistory: [],
        retryRemainingMs: 0,
        elapsedMs: 0,
        lastError: null,
        note: null,
        createdAt: minsAgo(4),
        chunkMap: chunkMapFor(48, 0, null),
    },
    {
        id: 'u4',
        name: 'north-ridge-survey-RAW.zip',
        kind: 'archive',
        sizeBytes: 36 * GB,
        chunkSizeBytes: 32 * MB,
        totalChunks: 1152,
        confirmedChunks: 410,
        bytesConfirmed: 12.8 * GB,
        inflightBytes: 0,
        bytesDiscarded: 96 * MB,
        bytesProtected: 12.8 * GB,
        status: 'paused',
        destination: 'Research / Imagery',
        sessionUri: 'https://drive.googleapis.com/upload/resumable/session/EF56def',
        driveFileId: null,
        checksum: '7c4a1f9e2b6d8a3c5f0e7b4d9a2c6f8e',
        resumeCount: 2,
        crashRecoveries: 0,
        throughputBps: 0,
        throughputHistory: [3, 4, 5, 4, 3, 2],
        retryRemainingMs: 0,
        elapsedMs: 22 * 60_000,
        lastError: null,
        note: null,
        createdAt: hoursAgo(2.4),
        chunkMap: chunkMapFor(64, 23, null),
    },
    {
        id: 'u5',
        name: 'kyiv_render_master.mov',
        kind: 'video',
        sizeBytes: 74 * GB,
        chunkSizeBytes: 64 * MB,
        totalChunks: 1184,
        confirmedChunks: 1184,
        bytesConfirmed: 74 * GB,
        inflightBytes: 0,
        bytesDiscarded: 0,
        bytesProtected: 74 * GB,
        status: 'verifying',
        destination: 'Field Uploads / Kyiv',
        sessionUri: 'https://drive.googleapis.com/upload/resumable/session/GH78ghi',
        driveFileId: null,
        checksum: '1a5e8c3f6b9d2a7c4f1e8b5d0a3c9f6e',
        resumeCount: 1,
        crashRecoveries: 0,
        throughputBps: 0,
        throughputHistory: [8, 9, 10, 9, 8],
        retryRemainingMs: 0,
        elapsedMs: 34 * 60_000,
        lastError: null,
        note: 'Final cut',
        createdAt: hoursAgo(3.1),
        chunkMap: chunkMapFor(64, 64, null),
    },
    {
        id: 'u6',
        name: 'proxies_day05.zip',
        kind: 'archive',
        sizeBytes: 17 * GB,
        chunkSizeBytes: 16 * MB,
        totalChunks: 1088,
        confirmedChunks: 1088,
        bytesConfirmed: 17 * GB,
        inflightBytes: 0,
        bytesDiscarded: 48 * MB,
        bytesProtected: 17 * GB,
        status: 'completed',
        destination: 'Field Uploads',
        sessionUri: null,
        driveFileId: '1a2b3c4d5e6f7g8h9i0j',
        checksum: '4d9a2f7c1e6b3d8a5c0f7e4b9d2a6c3f',
        resumeCount: 4,
        crashRecoveries: 1,
        throughputBps: 0,
        throughputHistory: [4, 5, 6, 5, 4, 3, 2],
        retryRemainingMs: 0,
        elapsedMs: 19 * 60_000,
        lastError: null,
        note: null,
        createdAt: hoursAgo(6),
        chunkMap: chunkMapFor(48, 48, null),
    },
    {
        id: 'u7',
        name: 'weather-composite.nc',
        kind: 'dataset',
        sizeBytes: 28 * GB,
        chunkSizeBytes: 32 * MB,
        totalChunks: 896,
        confirmedChunks: 896,
        bytesConfirmed: 28 * GB,
        inflightBytes: 0,
        bytesDiscarded: 32 * MB,
        bytesProtected: 28 * GB,
        status: 'completed',
        destination: 'Research / Cold Storage',
        sessionUri: null,
        driveFileId: '2b3c4d5e6f7g8h9i0j1k',
        checksum: '8e3b6d1a4f9c2e7b0d5a8c3f6e9b2d7a',
        resumeCount: 0,
        crashRecoveries: 0,
        throughputBps: 0,
        throughputHistory: [6, 7, 6, 5],
        retryRemainingMs: 0,
        elapsedMs: 12 * 60_000,
        lastError: null,
        note: null,
        createdAt: hoursAgo(9),
        chunkMap: chunkMapFor(40, 40, null),
    },
    {
        id: 'u8',
        name: 'laptop_full_2026-04-11.sparsebundle',
        kind: 'archive',
        sizeBytes: 512 * GB,
        chunkSizeBytes: 128 * MB,
        totalChunks: 4096,
        confirmedChunks: 1290,
        bytesConfirmed: 161 * GB,
        inflightBytes: 0,
        bytesDiscarded: 384 * MB,
        bytesProtected: 161 * GB,
        status: 'failed',
        destination: 'Research / Cold Storage',
        sessionUri: 'https://drive.googleapis.com/upload/resumable/session/IJ90jkl',
        driveFileId: null,
        checksum: '6f1c4a8e2b7d5a0c9f3e6b1d8a4c7f2e',
        resumeCount: 9,
        crashRecoveries: 2,
        throughputBps: 0,
        throughputHistory: [7, 8, 6, 5, 3, 1, 0],
        retryRemainingMs: 0,
        elapsedMs: 96 * 60_000,
        lastError: 'Drive quota check failed after 5 retries',
        note: 'Overnight backup',
        createdAt: hoursAgo(14),
        chunkMap: chunkMapFor(64, 20, 'f'),
    },
    {
        id: 'u9',
        name: 'contact_sheet.jpg',
        kind: 'image',
        sizeBytes: 24 * MB,
        chunkSizeBytes: 8 * MB,
        totalChunks: 3,
        confirmedChunks: 0,
        bytesConfirmed: 0,
        inflightBytes: 0,
        bytesDiscarded: 0,
        bytesProtected: 0,
        status: 'canceled',
        destination: 'Research / Imagery',
        sessionUri: null,
        driveFileId: null,
        checksum: '—',
        resumeCount: 0,
        crashRecoveries: 0,
        throughputBps: 0,
        throughputHistory: [],
        retryRemainingMs: 0,
        elapsedMs: 40_000,
        lastError: null,
        note: 'Wrong destination — canceled by user',
        createdAt: hoursAgo(20),
        chunkMap: chunkMapFor(3, 0, null),
    },
];

export const events: EventDTO[] = [
    { id: 'e1', uploadId: 'u1', level: 'info', message: 'Chunk 941 confirmed by Drive', detail: null, createdAt: minsAgo(1) },
    { id: 'e2', uploadId: 'u2', level: 'warn', message: 'Connection reset — holding position at chunk 640', detail: 'Retrying with exponential backoff', createdAt: minsAgo(3) },
    { id: 'e3', uploadId: 'u5', level: 'info', message: 'All chunks confirmed — comparing checksums', detail: null, createdAt: minsAgo(6) },
    { id: 'e4', uploadId: 'u3', level: 'info', message: 'Queued behind 2 active transfers', detail: null, createdAt: minsAgo(4) },
    { id: 'e5', uploadId: 'u1', level: 'success', message: 'Resumed after a 12s dropout — nothing re-sent', detail: null, createdAt: minsAgo(18) },
    { id: 'e6', uploadId: 'u8', level: 'error', message: 'Drive quota check failed after 5 retries', detail: 'Tap to retry manually', createdAt: hoursAgo(2) },
    { id: 'e7', uploadId: 'u6', level: 'success', message: 'proxies_day05.zip verified and marked complete', detail: 'Checksum matched Drive', createdAt: hoursAgo(3) },
    { id: 'e8', uploadId: 'u7', level: 'success', message: 'weather-composite.nc verified and marked complete', detail: null, createdAt: hoursAgo(4) },
    { id: 'e9', uploadId: 'u4', level: 'info', message: 'Paused by user', detail: null, createdAt: hoursAgo(2.4) },
    { id: 'e10', uploadId: 'u9', level: 'info', message: 'Canceled by user before any bytes were sent', detail: null, createdAt: hoursAgo(20) },
    { id: 'e11', uploadId: null, level: 'info', message: 'Journal replayed after restart — 3 sessions recovered', detail: null, createdAt: hoursAgo(23) },
    { id: 'e12', uploadId: 'u1', level: 'warn', message: 'Signal dropped to one bar — auto-resume standing by', detail: null, createdAt: hoursAgo(25) },
];

export const settings: SettingsDTO = {
    networkQuality: 'good',
    autoResume: true,
    verifyChecksums: true,
    maxConcurrent: 3,
    bandwidthCapMbps: 0,
    timeScale: 100,
    sessionsRecovered: 14,
    netHistory: [3, 5, 4, 6, 7, 5, 8, 9, 7, 6, 8, 10, 9, 8, 9, 7, 9, 10, 9, 8],
    userName: 'Mara Okonkwo',
    userEmail: 'mara.okonkwo@gmail.com',
    avatarSeed: 'MO',
    driveFolder: 'Field Uploads',
    storageUsedBytes: 812 * GB,
    storageTotalBytes: 2048 * GB,
};

export function stats() {
    const totalBytes = uploads.reduce((a, u) => a + u.sizeBytes, 0);
    const confirmedBytes = uploads.reduce((a, u) => a + u.bytesConfirmed, 0);
    const activeCount = uploads.filter((u) => !['completed', 'canceled'].includes(u.status)).length;
    const completedCount = uploads.filter((u) => u.status === 'completed').length;
    const protectedBytes = uploads.reduce((a, u) => a + u.bytesProtected, 0);
    const discardedBytes = uploads.reduce((a, u) => a + u.bytesDiscarded, 0);
    const throughputBps = uploads.filter((u) => u.status === 'uploading').reduce((a, u) => a + u.throughputBps, 0);
    const resumeCount = uploads.reduce((a, u) => a + u.resumeCount, 0);
    return { totalBytes, confirmedBytes, activeCount, completedCount, protectedBytes, discardedBytes, throughputBps, resumeCount };
}

export const LINKS: { id: NetworkQuality; label: string; sub: string; bps: number; loss: string; bars: number; emoji: string }[] = [
    { id: 'fiber', label: 'Fiber', sub: 'Hotel lobby, finally', bps: 62 * MB, loss: '≈0%', bars: 4, emoji: '🛜' },
    { id: 'good', label: 'Urban LTE', sub: 'City cell, the odd handover', bps: 14 * MB, loss: '5%/min', bars: 3, emoji: '📶' },
    { id: 'poor', label: 'Rural 3G', sub: 'One bar behind a hill', bps: 2.4 * MB, loss: '28%/min', bars: 2, emoji: '🏔️' },
    { id: 'edge', label: 'Satellite', sub: 'Long waits, heavy loss', bps: 420 * 1024, loss: '62%/min', bars: 1, emoji: '🛰️' },
    { id: 'offline', label: 'No signal', sub: 'Queue held, nothing lost', bps: 0, loss: '—', bars: 0, emoji: '🚫' },
];

/* ------------------------------------------------------------ file picker mock filesystem */

export type FsFile = { id: string; name: string; kind: 'video' | 'audio' | 'dataset' | 'archive' | 'image' | 'doc'; size: number; modified: string };
export type FsFolder = { id: string; name: string; folders?: FsFolder[]; files?: FsFile[] };
export type Volume = { id: string; name: string; type: 'internal' | 'card' | 'ssd' | 'cloud'; capacity: number; used: number; root: FsFolder };

const f = (name: string, kind: FsFile['kind'], size: number, modified: string): FsFile => ({ id: `${name}-${size}`, name, kind, size, modified });

export const VOLUMES: Volume[] = [
    {
        id: 'sd',
        name: 'SANDISK EXTREME',
        type: 'card',
        capacity: 1024 * GB,
        used: 918 * GB,
        root: {
            id: 'sd-root',
            name: 'SANDISK EXTREME',
            folders: [
                {
                    id: 'sd-dcim',
                    name: 'DCIM · A-cam',
                    files: [
                        f('A001_C012_0417RB.mov', 'video', 214 * GB, 'Today, 06:42'),
                        f('A001_C013_0417RB.mov', 'video', 168 * GB, 'Today, 07:15'),
                        f('A001_C014_0417RB.mov', 'video', 96 * GB, 'Today, 08:03'),
                        f('A001_C015_0417RB.mov', 'video', 52 * GB, 'Today, 08:44'),
                    ],
                },
                {
                    id: 'sd-bcam',
                    name: 'DCIM · B-cam',
                    files: [
                        f('B001_C004_0417XY.mov', 'video', 121 * GB, 'Today, 06:50'),
                        f('B001_C005_0417XY.mov', 'video', 88 * GB, 'Today, 07:32'),
                    ],
                },
                {
                    id: 'sd-proxies',
                    name: 'Proxies',
                    files: [
                        f('proxies_day04.zip', 'archive', 14 * GB, 'Yesterday, 22:10'),
                        f('proxies_day05.zip', 'archive', 17 * GB, 'Today, 09:01'),
                    ],
                },
            ],
            files: [f('CARD_MANIFEST.txt', 'doc', 12 * 1024, 'Today, 09:02')],
        },
    },
    {
        id: 'ssd',
        name: 'Field SSD T7',
        type: 'ssd',
        capacity: 2048 * GB,
        used: 1340 * GB,
        root: {
            id: 'ssd-root',
            name: 'Field SSD T7',
            folders: [
                {
                    id: 'ssd-audio',
                    name: 'Interviews',
                    files: [
                        f('int_014_valentyna_multitrack.wav', 'audio', 41 * GB, 'Yesterday, 18:22'),
                        f('int_015_bohdan_multitrack.wav', 'audio', 33 * GB, 'Yesterday, 20:04'),
                        f('int_016_field_ambience.wav', 'audio', 12 * GB, 'Today, 05:58'),
                        f('interview_masters_2026.wav.zst', 'audio', 78 * GB, 'Today, 10:20'),
                    ],
                },
                {
                    id: 'ssd-data',
                    name: 'Sensor exports',
                    files: [
                        f('sensor-array-2026-Q1.parquet.tar', 'dataset', 240 * GB, '3 days ago'),
                        f('glacier-station-04.parquet', 'dataset', 61 * GB, '3 days ago'),
                        f('weather-composite.nc', 'dataset', 28 * GB, '4 days ago'),
                    ],
                },
                {
                    id: 'ssd-stills',
                    name: 'Stills',
                    files: [
                        f('north-ridge-survey-RAW.zip', 'archive', 36 * GB, 'Yesterday, 14:40'),
                        f('portraits_selects.zip', 'archive', 9 * GB, 'Yesterday, 15:12'),
                        f('contact_sheet.jpg', 'image', 24 * MB, 'Yesterday, 15:20'),
                    ],
                },
            ],
        },
    },
    {
        id: 'mac',
        name: 'Macintosh HD',
        type: 'internal',
        capacity: 1024 * GB,
        used: 712 * GB,
        root: {
            id: 'mac-root',
            name: 'Macintosh HD',
            folders: [
                {
                    id: 'mac-projects',
                    name: 'Projects',
                    folders: [
                        {
                            id: 'mac-kyiv',
                            name: 'Kyiv 2026',
                            files: [
                                f('kyiv_timeline_v14.prproj', 'doc', 840 * MB, 'Today, 11:02'),
                                f('kyiv_render_master.mov', 'video', 74 * GB, 'Today, 11:40'),
                                f('kyiv_stems.wav', 'audio', 18 * GB, 'Today, 11:44'),
                            ],
                        },
                        {
                            id: 'mac-ridge',
                            name: 'North Ridge',
                            files: [
                                f('ridge_cut_v3.mov', 'video', 46 * GB, '2 days ago'),
                                f('ridge_notes.md', 'doc', 84 * 1024, '2 days ago'),
                            ],
                        },
                    ],
                },
                {
                    id: 'mac-backups',
                    name: 'Backups',
                    files: [
                        f('laptop_full_2026-04-11.sparsebundle', 'archive', 512 * GB, '5 days ago'),
                        f('keychain_export.enc', 'doc', 3 * MB, '5 days ago'),
                    ],
                },
            ],
        },
    },
];

export function findFolder(root: FsFolder, id: string): FsFolder | null {
    if (root.id === id) return root;
    for (const sub of root.folders ?? []) {
        const hit = findFolder(sub, id);
        if (hit) return hit;
    }
    return null;
}

export function pathTo(root: FsFolder, id: string): FsFolder[] {
    if (root.id === id) return [root];
    for (const sub of root.folders ?? []) {
        const p = pathTo(sub, id);
        if (p.length) return [root, ...p];
    }
    return [];
}

export const DRIVE_FOLDERS = [
    { name: 'Field Uploads', hint: 'Default destination', color: 'grass' },
    { name: 'Field Uploads / Kyiv', hint: 'Shared with 3 editors', color: 'sky' },
    { name: 'Research / Cold Storage', hint: 'Archive tier', color: 'grape' },
    { name: 'Research / Imagery', hint: 'Survey frames', color: 'tangerine' },
    { name: 'Newsroom Dropbox', hint: 'Desk can pull directly', color: 'coral' },
];

export const FOLDER_TONE: Record<string, string> = {
    grass: 'tone-grass',
    sky: 'tone-sky',
    grape: 'tone-grape',
    tangerine: 'tone-tangerine',
    coral: 'tone-coral',
};

export function pickChunkSize(sizeBytes: number): number {
    if (sizeBytes > 32 * GB) return 128 * MB;
    if (sizeBytes > 4 * GB) return 64 * MB;
    if (sizeBytes > 256 * MB) return 32 * MB;
    return 8 * MB;
}
