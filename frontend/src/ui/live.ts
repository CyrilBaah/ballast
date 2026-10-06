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
// so this module doesn't expose pause/resume. It does expose a client-side
// queue (enqueueUploads/advanceQueue) on top of that single-active-upload
// gate: picking several files enqueues all of them as synthetic 'queued'
// entries and starts the real backend upload for one at a time, advancing
// to the next the moment the current one reaches a terminal state.

import { GetStatus as authGetStatus, SignIn as authSignIn, SignOut as authSignOut } from '../api/auth';
import type { AuthStatus } from '../api/auth';
import { ListFolders, GetStorageQuota } from '../api/drive';
import type { DriveFolder, StorageQuota } from '../api/drive';
import { PickLocal, PickLocalMultiple } from '../api/files';
import type { LocalFileRef } from '../api/files';
import { Start, GetRecoverable, ConfirmRestart, Cancel, Delete, Retry, ListRecent } from '../api/upload';
import { AnswerModelDownload, GetSettings as getCaptionSettings, ShowLocalCopy } from '../api/captions';
import type { CaptionJob } from '../api/captions';
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
    /** The video's captioning state (Feature 005), or null if it has none. */
    caption: CaptionJob | null;
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
            caption: null,
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
        void advanceQueue();
    });
    EventsOn('captions:updated', (c: CaptionJob) => {
        const id = String(c.uploadId);
        const u = uploadsById.get(id);
        if (!u) return;
        const before = u.caption?.status;
        u.caption = c;
        if (before !== 'done' && c.status === 'done' && c.driveFileLink) {
            pushActivity('success', `Captions for ${u.name} are in Drive`);
        }
        notify();
    });
    EventsOn('captions:consent-needed', (p: { modelSizeBytes: number }) => {
        captionConsent = { sizeBytes: p.modelSizeBytes };
        notify();
    });
    EventsOn('upload:failed', (p: { id: number; reason: string }) => {
        const id = String(p.id);
        lastSample.delete(id);
        const u = upsert(id, { status: 'failed', failureReason: p.reason, throughputBps: 0 });
        pushActivity('error', `${u.name} failed — ${toPlainLanguage(p.reason)}`);
        notify();
        void advanceQueue();
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
            // Recorded right away, not just once ListRecent resolves below --
            // otherwise a file picked in the gap between these two awaits
            // would see an empty uploadsById, sail past hasActiveUpload()/
            // findInFlight(), and hit the backend's single-active-upload
            // gate instead (the leftover row is already non-terminal there).
            upsert(
                String(recoverable.id),
                {
                    name: recoverable.fileName,
                    kind: fileKind(recoverable.fileName),
                    sizeBytes: recoverable.totalBytes,
                    bytesConfirmed: recoverable.bytesSent,
                    status: mapStatus(recoverable.status),
                    awaitingConfirmationReason: recoverable.awaitingConfirmationReason || null,
                },
                recoverable.fileName,
            );
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
                caption: item.caption ?? null,
            });
        }
        uploadsOrder = list.map((item) => String(item.id));
        // The worker's consent-needed event can fire before this page is
        // listening (e.g. right after a relaunch), so also derive the
        // prompt from what's on record.
        if (list.some((item) => item.caption?.phase === 'awaiting_consent')) {
            const settings = await getCaptionSettings();
            if (settings.modelConsent === 'unasked') captionConsent = { sizeBytes: settings.modelSizeBytes };
        }
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

// Mirrors the backend's own gate (storage.nonTerminalStatuses: in_progress,
// paused, awaiting_confirmation) -- deliberately excludes 'queued', since a
// queued entry hasn't been submitted to the backend yet and must not block
// the next queue item from starting.
const BACKEND_ACTIVE_STATUSES: UploadStatus[] = ['uploading', 'reconnecting', 'awaiting_confirmation'];

function hasActiveUpload(): boolean {
    for (const id of uploadsOrder) {
        const u = uploadsById.get(id);
        if (u && BACKEND_ACTIVE_STATUSES.includes(u.status)) return true;
    }
    return false;
}

export async function pickFile(): Promise<LocalFileRef | null> {
    return PickLocal();
}

export async function pickFiles(): Promise<LocalFileRef[]> {
    return PickLocalMultiple();
}

export async function listDriveFolders(parentId: string): Promise<DriveFolder[]> {
    return ListFolders(parentId);
}

interface QueuedUpload {
    syntheticId: string;
    file: LocalFileRef;
    folder: { id: string; name: string };
}

const uploadQueue: QueuedUpload[] = [];
let syntheticSeq = 0;

// Same set BACKEND_ACTIVE_STATUSES uses, plus 'queued': a file waiting in
// the client-side queue is just as "taken" as one actively uploading, so a
// second pick of the same destination must be rejected while either holds.
const IN_FLIGHT_STATUSES: UploadStatus[] = ['queued', 'uploading', 'reconnecting', 'awaiting_confirmation'];

// "Same file" is judged by destination identity (drive folder + file name),
// which is exactly what LiveUpload.destination/name already record -- this
// also catches a leftover in-progress upload hydrated from ListRecent after
// a restart, with no extra bookkeeping needed.
function findInFlight(fileName: string, folderName: string): LiveUpload | undefined {
    for (const id of uploadsOrder) {
        const u = uploadsById.get(id);
        if (u && u.name === fileName && u.destination === folderName && IN_FLIGHT_STATUSES.includes(u.status)) {
            return u;
        }
    }
    return undefined;
}

function nextSyntheticId(): string {
    syntheticSeq += 1;
    return `queued-${Date.now()}-${syntheticSeq}`;
}

// Replaces a synthetic queue entry's map key/order slot with the real
// backend id, in place, so the row doesn't jump position or flicker.
function replaceId(oldId: string, newId: string): void {
    const existing = uploadsById.get(oldId);
    if (!existing) return;
    uploadsById.delete(oldId);
    existing.id = newId;
    uploadsById.set(newId, existing);
    const idx = uploadsOrder.indexOf(oldId);
    if (idx !== -1) uploadsOrder[idx] = newId;
}

async function startUpload(file: LocalFileRef, folder: { id: string; name: string }, syntheticId: string): Promise<{ ok: boolean; error?: string; requeued?: boolean }> {
    if (hasActiveUpload()) {
        return { ok: false, error: 'Finish or cancel the current upload before starting another.' };
    }
    try {
        const id = await Start(file.path, folder.id || 'root', folder.name || 'My Drive');
        defaultFolder = folder;
        const realId = String(id);
        replaceId(syntheticId, realId);
        upsert(
            realId,
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
        pushActivity('info', `${file.name} started uploading`);
        notify();
        return { ok: true };
    } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        if (message.toLowerCase().includes('already in progress')) {
            // The backend's single-active-upload slot was still held by a
            // leftover upload at the exact instant this one tried to claim
            // it (a narrow hydration-timing race) -- put it back at the
            // front of the queue instead of failing it. advanceQueue gets
            // re-invoked the moment that other upload reaches a terminal
            // state (its upload:complete/upload:failed handler already
            // calls it), so this file starts automatically once it's clear.
            uploadQueue.unshift({ syntheticId, file, folder });
            pushActivity('info', `${file.name} is waiting for the current upload to finish`);
            notify();
            return { ok: false, error: message, requeued: true };
        }
        upsert(syntheticId, { status: 'failed', failureReason: message });
        pushActivity('error', `${file.name} could not start — ${toPlainLanguage(message)}`);
        notify();
        return { ok: false, error: message };
    }
}

let advancingQueue = false;

// Drains the queue one item at a time: starts the next queued file's real
// backend upload only once nothing is backend-active, then relies on its
// caller sites (a completion/failure event, or cancelUpload resolving) to
// call this again for the item after that.
async function advanceQueue(): Promise<void> {
    if (advancingQueue || hasActiveUpload() || !uploadQueue.length) return;
    advancingQueue = true;
    const next = uploadQueue.shift()!;
    const outcome = await startUpload(next.file, next.folder, next.syntheticId);
    advancingQueue = false;
    // A requeued item means the backend slot is still genuinely held by
    // someone else -- retrying immediately would just busy-loop on the same
    // conflict. Wait for that upload's terminal event to call advanceQueue
    // again instead. Any other failure leaves the backend free, so keep draining.
    if (!outcome.requeued) void advanceQueue();
}

export function enqueueUploads(files: LocalFileRef[], folder: { id: string; name: string }): void {
    const folderName = folder.name || 'My Drive';
    for (const file of files) {
        if (findInFlight(file.name, folderName)) {
            pushActivity('warn', `${file.name} is already uploading to ${folderName} — skipped duplicate pick`);
            continue;
        }
        const syntheticId = nextSyntheticId();
        uploadQueue.push({ syntheticId, file, folder });
        upsert(
            syntheticId,
            {
                name: file.name,
                kind: fileKind(file.name),
                sizeBytes: file.sizeBytes,
                bytesConfirmed: 0,
                status: 'queued',
                destination: folderName,
            },
            file.name,
        );
    }
    notify();
    void advanceQueue();
}

export async function cancelUpload(id: string): Promise<void> {
    // A queued item hasn't been submitted to the backend yet (no real id to
    // cancel there) -- just drop it from the local queue.
    const queueIdx = uploadQueue.findIndex((q) => q.syntheticId === id);
    if (queueIdx !== -1) {
        uploadQueue.splice(queueIdx, 1);
        upsert(id, { status: 'canceled' });
        notify();
        return;
    }
    await Cancel(Number(id));
    void advanceQueue();
}

export async function confirmRestartUpload(id: string): Promise<void> {
    await ConfirmRestart(Number(id));
}

// Starts a brand-new upload of a cancelled transfer's same file/destination
// (UploadRetry, app.go) -- a distinct row alongside the old cancelled one in
// History, not a resurrection of it, since Drive already closed that session.
// Rejects the same way any other Start attempt does if something else is
// already active (surfaced to the caller as a plain-language toast, same as
// cancel/restart) -- retrying is a single deliberate click, not something
// worth building a queue-and-wait path for.
export async function retryUpload(id: string): Promise<void> {
    const old = uploadsById.get(id);
    const newId = await Retry(Number(id));
    const realId = String(newId);
    const name = old?.name ?? `Upload #${realId}`;
    upsert(
        realId,
        {
            name,
            kind: fileKind(name),
            sizeBytes: old?.sizeBytes ?? 0,
            bytesConfirmed: 0,
            status: 'uploading',
            destination: old?.destination ?? 'My Drive',
        },
        name,
    );
    pushActivity('info', `${name} started uploading again`);
    notify();
}

// Permanently removes a terminal (completed/failed/canceled) upload from
// history. The backend rejects anything still active (storage.DeleteUpload),
// so this only ever needs to drop the row from local state, not reconcile
// it against a queue or in-flight transfer.
export async function deleteUpload(id: string): Promise<void> {
    await Delete(Number(id));
    uploadsById.delete(id);
    uploadsOrder = uploadsOrder.filter((existingId) => existingId !== id);
    notify();
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

/** Set while the one-time speech-model download prompt should show (FR-019). */
export let captionConsent: { sizeBytes: number } | null = null;

export async function answerCaptionConsent(accept: boolean): Promise<void> {
    captionConsent = null;
    notify();
    await AnswerModelDownload(accept);
}

export async function showCaptionLocalCopy(id: string): Promise<void> {
    await ShowLocalCopy(Number(id));
}
