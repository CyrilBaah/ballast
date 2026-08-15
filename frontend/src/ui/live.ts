// Bridges the real Wails/Drive backend (../api/*.ts, which wrap the
// generated wailsjs bindings) into the redesigned UI's data shape. This
// replaces mock-data.ts's uploads/stats/settings for the views that are
// wired to real data: shell (title bar, sidebar), home, transfers,
// settings, onboarding, and the file picker.
//
// Journal and Field Lab stay on mock-data.ts on purpose — there's no real
// per-chunk event log or "break it on purpose" endpoint to back them.
//
// The backend allows exactly one active upload at a time (Upload.Start
// rejects a second call while one is in_progress/paused/awaiting_confirmation)
// and has no manual-pause endpoint (only automatic retry-pause and Cancel),
// so this module doesn't expose pause/resume or a multi-upload queue.

import { GetStatus as authGetStatus, SignIn as authSignIn, SignOut as authSignOut } from '../api/auth';
import type { AuthStatus } from '../api/auth';
import { ListFolders, GetStorageQuota } from '../api/drive';
import type { DriveFolder, StorageQuota } from '../api/drive';
import { PickLocal } from '../api/files';
import type { LocalFileRef } from '../api/files';
import { Start, GetRecoverable, ConfirmRestart, Cancel, ListRecent } from '../api/upload';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { toPlainLanguage } from '../errors';
import type { UploadStatus } from './components';

export type { DriveFolder, LocalFileRef };

export interface LiveUpload {
    id: string;
    name: string;
    kind: string;
    sizeBytes: number;
    bytesConfirmed: number;
    status: UploadStatus;
    destination: string;
    driveFileLink: string | null;
    failureReason: string | null;
    awaitingConfirmationReason: string | null;
    throughputBps: number;
    startedAt: string;
}

export interface ActivityEntry {
    id: string;
    level: 'info' | 'warn' | 'error' | 'success';
    message: string;
    createdAt: string;
}

export const profile = {
    signedIn: false,
    email: '',
    name: '',
    pictureUrl: '',
};

export let storageQuota: StorageQuota | null = null;
export let defaultFolder: { id: string; name: string } = { id: '', name: 'My Drive' };
export function setDefaultFolder(folder: { id: string; name: string }): void {
    defaultFolder = folder;
}
export const activityLog: ActivityEntry[] = [];
export const netHistory: number[] = [];

const uploadsById = new Map<string, LiveUpload>();
let uploadsOrder: string[] = [];
const lastSample = new Map<string, { bytes: number; at: number }>();

let onChange: (() => void) | null = null;
export function subscribe(cb: () => void): void {
    onChange = cb;
}
function notify(): void {
    onChange?.();
}

function pushActivity(level: ActivityEntry['level'], message: string): void {
    activityLog.unshift({ id: `a-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`, level, message, createdAt: new Date().toISOString() });
    activityLog.length = Math.min(activityLog.length, 30);
}

const KIND_EXTENSIONS: Record<string, string> = {
    mov: 'video', mp4: 'video', mkv: 'video', avi: 'video', m4v: 'video',
    wav: 'audio', mp3: 'audio', m4a: 'audio', aac: 'audio', flac: 'audio',
    jpg: 'image', jpeg: 'image', png: 'image', gif: 'image', heic: 'image', tiff: 'image', webp: 'image',
    pdf: 'doc', doc: 'doc', docx: 'doc', txt: 'doc', md: 'doc', key: 'doc', ppt: 'doc', pptx: 'doc', xls: 'doc', xlsx: 'doc', csv: 'doc',
    parquet: 'dataset', nc: 'dataset', json: 'dataset', db: 'dataset', sqlite: 'dataset',
    zip: 'archive', rar: 'archive', '7z': 'archive', tar: 'archive', gz: 'archive', sparsebundle: 'archive',
};

export function fileKind(name: string): string {
    const ext = name.split('.').pop()?.toLowerCase() ?? '';
    return KIND_EXTENSIONS[ext] ?? 'archive';
}

function mapStatus(raw: string): UploadStatus {
    switch (raw) {
        case 'pending':
            return 'queued';
        case 'in_progress':
            return 'uploading';
        case 'paused':
            return 'reconnecting';
        case 'awaiting_confirmation':
            return 'awaiting_confirmation';
        case 'succeeded':
            return 'completed';
        case 'cancelled':
            return 'canceled';
        case 'failed':
        default:
            return 'failed';
    }
}

function upsert(id: string, patch: Partial<LiveUpload>, seedName?: string): LiveUpload {
    let u = uploadsById.get(id);
    if (!u) {
        u = {
            id,
            name: seedName ?? `Upload #${id}`,
            kind: fileKind(seedName ?? ''),
            sizeBytes: 0,
            bytesConfirmed: 0,
            status: 'queued',
            destination: 'My Drive',
            driveFileLink: null,
            failureReason: null,
            awaitingConfirmationReason: null,
            throughputBps: 0,
            startedAt: new Date().toISOString(),
        };
        uploadsById.set(id, u);
        uploadsOrder = [id, ...uploadsOrder];
    }
    Object.assign(u, patch);
    return u;
}

function recordSpeed(id: string, bytesConfirmed: number): void {
    const now = Date.now();
    const prev = lastSample.get(id);
    lastSample.set(id, { bytes: bytesConfirmed, at: now });
    if (!prev) return;
    const dtSec = (now - prev.at) / 1000;
    const delta = bytesConfirmed - prev.bytes;
    if (dtSec <= 0 || delta < 0) return;
    const u = uploadsById.get(id);
    if (u) u.throughputBps = delta / dtSec;
}

let eventsWired = false;
function wireUploadEvents(): void {
    if (eventsWired) return;
    eventsWired = true;
    EventsOn('upload:progress', (p: { id: number; bytesSent: number; totalBytes: number }) => {
        const id = String(p.id);
        recordSpeed(id, p.bytesSent);
        const u = upsert(id, { status: 'uploading', bytesConfirmed: p.bytesSent, sizeBytes: p.totalBytes });
        netHistory.push(u.throughputBps);
        netHistory.splice(0, Math.max(0, netHistory.length - 48));
        notify();
    });
    EventsOn('upload:paused', (p: { id: number }) => {
        const id = String(p.id);
        lastSample.delete(id);
        upsert(id, { status: 'reconnecting', throughputBps: 0 });
        notify();
    });
    EventsOn('upload:awaiting-confirmation', (p: { id: number; reason: string }) => {
        const id = String(p.id);
        lastSample.delete(id);
        upsert(id, { status: 'awaiting_confirmation', awaitingConfirmationReason: p.reason, throughputBps: 0 });
        notify();
    });
    EventsOn('upload:complete', (p: { id: number; driveFileLink: string }) => {
        const id = String(p.id);
        lastSample.delete(id);
        const u = upsert(id, { status: 'completed', driveFileLink: p.driveFileLink, throughputBps: 0 });
        pushActivity('success', `${u.name} confirmed by Google Drive`);
        void refreshStorageQuota();
        notify();
    });
    EventsOn('upload:failed', (p: { id: number; reason: string }) => {
        const id = String(p.id);
        lastSample.delete(id);
        const u = upsert(id, { status: 'failed', failureReason: p.reason, throughputBps: 0 });
        pushActivity('error', `${u.name} failed — ${toPlainLanguage(p.reason)}`);
        notify();
    });
}

function applyAuthStatus(status: AuthStatus): void {
    profile.signedIn = status.signedIn;
    profile.email = status.email ?? '';
    profile.name = status.name ?? '';
    profile.pictureUrl = status.pictureUrl ?? '';
    if (status.signedIn) {
        void hydrateAfterSignIn();
    } else {
        uploadsById.clear();
        uploadsOrder = [];
        storageQuota = null;
    }
    notify();
}

async function hydrateAfterSignIn(): Promise<void> {
    wireUploadEvents();
    try {
        // Runs first: if a leftover upload from a previous run needs to
        // resume or fall back to awaiting-confirmation, it happens here so
        // ListRecent below already reflects the corrected status.
        const recoverable = await GetRecoverable();
        if (recoverable) {
            pushActivity('info', `Picking up ${recoverable.fileName} where it left off`);
        }
    } catch {
        /* non-fatal */
    }
    try {
        const list = await ListRecent();
        uploadsById.clear();
        for (const item of list) {
            const id = String(item.id);
            uploadsById.set(id, {
                id,
                name: item.fileName,
                kind: fileKind(item.fileName),
                sizeBytes: item.totalBytes,
                bytesConfirmed: item.bytesSent,
                status: mapStatus(item.status),
                destination: item.driveFolderName,
                driveFileLink: item.driveFileLink ?? null,
                failureReason: item.failureReason ?? null,
                awaitingConfirmationReason: null,
                throughputBps: 0,
                startedAt: item.startedAt,
            });
        }
        uploadsOrder = list.map((item) => String(item.id));
    } catch {
        /* non-fatal -- live events still drive updates from here */
    }
    void refreshStorageQuota();
    notify();
}

async function refreshStorageQuota(): Promise<void> {
    try {
        storageQuota = await GetStorageQuota();
        notify();
    } catch {
        /* leave null; the UI hides the storage tile */
    }
}

let authWired = false;
export function initLive(onUpdate: () => void): void {
    onChange = onUpdate;
    if (!authWired) {
        authWired = true;
        EventsOn('auth:changed', (status: AuthStatus) => applyAuthStatus(status));
    }
    void authGetStatus()
        .then(applyAuthStatus)
        .catch(() => applyAuthStatus({ signedIn: false }));
}

export async function signIn(): Promise<{ ok: boolean; error?: string }> {
    try {
        const status = await authSignIn();
        applyAuthStatus(status);
        return { ok: status.signedIn };
    } catch (err) {
        return { ok: false, error: err instanceof Error ? err.message : String(err) };
    }
}

export async function signOut(): Promise<{ error?: string }> {
    try {
        await authSignOut();
        return {};
    } catch (err) {
        // Local state clears via the auth:changed event regardless of
        // whether server-side revocation succeeded (app.go always emits
        // it) -- surface the error without blocking the sign-out.
        return { error: err instanceof Error ? err.message : String(err) };
    }
}

const ACTIVE_STATUSES: UploadStatus[] = ['queued', 'uploading', 'reconnecting', 'awaiting_confirmation'];

export function hasActiveUpload(): boolean {
    for (const id of uploadsOrder) {
        const u = uploadsById.get(id);
        if (u && ACTIVE_STATUSES.includes(u.status)) return true;
    }
    return false;
}

export async function pickFile(): Promise<LocalFileRef | null> {
    return PickLocal();
}

export async function listDriveFolders(parentId: string): Promise<DriveFolder[]> {
    return ListFolders(parentId);
}

export async function startUpload(file: LocalFileRef, folder: { id: string; name: string }): Promise<{ ok: boolean; error?: string }> {
    if (hasActiveUpload()) {
        return { ok: false, error: 'Finish or cancel the current upload before starting another.' };
    }
    try {
        const id = await Start(file.path, folder.id || 'root', folder.name || 'My Drive');
        defaultFolder = folder;
        upsert(
            String(id),
            {
                name: file.name,
                kind: fileKind(file.name),
                sizeBytes: file.sizeBytes,
                bytesConfirmed: 0,
                status: 'uploading',
                destination: folder.name || 'My Drive',
            },
            file.name,
        );
        pushActivity('info', `${file.name} queued for upload`);
        notify();
        return { ok: true };
    } catch (err) {
        return { ok: false, error: err instanceof Error ? err.message : String(err) };
    }
}

export async function cancelUpload(id: string): Promise<void> {
    await Cancel(Number(id));
}

export async function confirmRestartUpload(id: string): Promise<void> {
    await ConfirmRestart(Number(id));
}

export function orderedUploads(): LiveUpload[] {
    return uploadsOrder.map((id) => uploadsById.get(id)).filter((u): u is LiveUpload => !!u);
}

export function liveStats() {
    let totalBytes = 0;
    let confirmedBytes = 0;
    let activeCount = 0;
    let completedCount = 0;
    let throughputBps = 0;
    for (const u of orderedUploads()) {
        totalBytes += u.sizeBytes;
        confirmedBytes += u.bytesConfirmed;
        if (u.status === 'completed') completedCount++;
        if (u.status !== 'completed' && u.status !== 'canceled') activeCount++;
        if (u.status === 'uploading') throughputBps += u.throughputBps;
    }
    return { totalBytes, confirmedBytes, activeCount, completedCount, throughputBps };
}
