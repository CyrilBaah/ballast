import './styles/tokens.css';
import './style.css';
import './app.css';

import { renderSignIn } from './screens/signin';
import { renderPicker } from './screens/picker';
import { renderProgress } from './screens/progress';
import { renderHistory } from './screens/history';
import { GetRecoverable, GetStatus as UploadGetStatus, type RecoverableUpload } from './api/upload';
import { GetStatus as AuthGetStatus, type AuthStatus } from './api/auth';
import { GetStorageQuota } from './api/drive';
import type { LocalFileRef } from './api/files';
import { formatBytes } from './format';

const app = document.querySelector<HTMLElement>('#app')!;

const WAVE_LAYER_SVG = `
    <svg class="wave-layer" viewBox="0 0 880 500" preserveAspectRatio="none" aria-hidden="true">
        <path class="w1" d="M-20,120 C120,70 220,170 360,120 C500,70 600,170 740,120 C820,95 860,110 900,100" />
        <path class="w2" d="M-20,230 C100,190 240,270 380,230 C520,190 640,270 780,230 C840,215 870,225 900,220" />
        <path class="w3" d="M-20,360 C140,320 260,400 400,360 C540,320 660,400 800,360 C850,345 875,352 900,348" />
        <circle cx="120" cy="88" r="4" fill="var(--color-accent)" />
        <circle cx="700" cy="205" r="3.5" fill="var(--color-success)" />
        <circle cx="260" cy="340" r="3" fill="var(--color-warning)" />
        <circle cx="620" cy="380" r="4" fill="var(--filetype-image)" />
    </svg>
`;

type ShellScreen = 'upload' | 'history';

interface ShellHandle {
    content: HTMLElement;
    setActiveNav: (screen: ShellScreen) => void;
}

let teardown: (() => void) | null = null; // sign-in screen's teardown
let screenTeardown: (() => void) | null = null; // active screen inside the shell's .content
let shell: ShellHandle | null = null;
let currentEmail: string | undefined; // kept for the picker's "Signed in as" line

// The upload the app last knew to be non-terminal, if any -- set when a
// new upload starts (onUploadStarted) or one is resumed at launch
// (routeAfterSignIn's recoverable branch), cleared once the progress
// screen observes a terminal status (renderProgress's onTerminal).
let activeUpload: { id: number; fileName: string } | null = null;

// Only awaiting_confirmation actually strands the user if they navigate
// away from it: it's the sole status that halts forward progress until
// the user picks Restart or Cancel on the progress screen (the only place
// those controls are shown). in_progress and paused keep moving on their
// own in the background and are already fully visible from History (per
// User Story 3 / SC-003, frontend/tests/upload-history.spec.ts) -- History
// must NOT redirect away from those, or it becomes unreachable for the
// entire duration of every upload. Upload (the picker) redirects on any
// non-terminal status instead, since starting a second upload is never
// valid while one exists (UploadStart: ErrUploadAlreadyInProgress) --
// sending the user back to the existing upload beats surfacing that raw
// backend error.
const NON_TERMINAL_UPLOAD_STATUSES = new Set(['pending', 'in_progress', 'paused', 'awaiting_confirmation']);

// History only needs to redirect for the status that actually blocks
// forward progress -- in_progress/paused rows stay visible and
// live-updating from History itself (SC-003), so redirecting away from
// them would defeat that.
const HISTORY_BLOCKING_STATUSES = new Set(['awaiting_confirmation']);

// Shows the progress screen for the tracked active upload if its status is
// in blockingStatuses, and reports whether it did so. Deliberately reads
// status via UploadGetStatus (a plain DB read) rather than
// UploadGetRecoverable, which has the side effect of re-launching a resume
// goroutine for a paused upload -- calling that on every nav click would
// risk spawning duplicate goroutines racing to send chunks to the same
// Drive session.
async function redirectToActiveUpload(blockingStatuses: ReadonlySet<string>): Promise<boolean> {
    if (!activeUpload) return false;
    const upload = activeUpload;

    let status: { status: string };
    try {
        status = await UploadGetStatus(upload.id);
    } catch (err) {
        // Can't confirm its status -- don't strand the user on a broken
        // redirect loop, just let the requested navigation proceed.
        console.error('Upload.GetStatus failed', err);
        activeUpload = null;
        return false;
    }

    if (!NON_TERMINAL_UPLOAD_STATUSES.has(status.status)) {
        activeUpload = null;
        return false;
    }
    if (!blockingStatuses.has(status.status)) {
        return false;
    }

    if (!shell) return false; // signed out while the status check was in flight
    screenTeardown?.();
    shell.setActiveNav('upload');
    screenTeardown = renderProgress({
        container: shell.content,
        uploadId: upload.id,
        fileName: upload.fileName,
        onTerminal: () => {
            if (activeUpload?.id === upload.id) activeUpload = null;
        },
    });
    return true;
}

function showSignIn() {
    shell = null;
    screenTeardown?.();
    screenTeardown = null;
    teardown?.();
    teardown = renderSignIn({
        container: app,
        onSignedIn: (status: AuthStatus) => {
            teardown = null;
            void routeAfterSignIn(status);
        },
    });
}

// Mounts the persistent left sidebar shell (Upload/History nav plus the
// bottom account-status row) once, wrapping every signed-in screen
// (research.md §6). Sign-in itself stays full-screen, outside this shell.
function mountShell(): ShellHandle {
    app.innerHTML = `
        <div class="shell">
            ${WAVE_LAYER_SVG}
            <nav class="sidebar" aria-label="Main">
                <div class="sidebar-brand">Ballast</div>
                <ul class="sidebar-nav">
                    <li><button type="button" id="nav-upload" class="sidebar-nav-item">Upload</button></li>
                    <li><button type="button" id="nav-history" class="sidebar-nav-item">History</button></li>
                </ul>
                <div class="sidebar-account">
                    <div id="sidebar-avatar" class="sidebar-avatar"></div>
                    <div class="sidebar-account-info">
                        <div id="sidebar-name" class="sidebar-name"></div>
                        <div id="sidebar-storage" class="sidebar-storage" hidden></div>
                    </div>
                </div>
            </nav>
            <div id="shell-content" class="content"></div>
        </div>
    `;

    const content = app.querySelector<HTMLElement>('#shell-content')!;
    const navUpload = app.querySelector<HTMLButtonElement>('#nav-upload')!;
    const navHistory = app.querySelector<HTMLButtonElement>('#nav-history')!;

    function setActiveNav(screen: ShellScreen) {
        navUpload.classList.toggle('active', screen === 'upload');
        navHistory.classList.toggle('active', screen === 'history');
        navUpload.setAttribute('aria-current', screen === 'upload' ? 'page' : 'false');
        navHistory.setAttribute('aria-current', screen === 'history' ? 'page' : 'false');
    }

    navUpload.addEventListener('click', () => void showUpload());
    navHistory.addEventListener('click', () => void showHistory());

    void renderAccountStatus(app);

    return { content, setActiveNav };
}

function initialsFrom(name: string | undefined, email: string | undefined): string {
    const source = (name && name.trim()) || (email && email.trim()) || '';
    return source ? source[0].toUpperCase() : '?';
}

async function renderAccountStatus(root: HTMLElement) {
    const avatarEl = root.querySelector<HTMLElement>('#sidebar-avatar')!;
    const nameEl = root.querySelector<HTMLElement>('#sidebar-name')!;
    const storageEl = root.querySelector<HTMLElement>('#sidebar-storage')!;

    let status: AuthStatus;
    try {
        status = await AuthGetStatus();
    } catch (err) {
        console.error('Auth.GetStatus failed', err);
        return;
    }

    nameEl.textContent = status.name || status.email || '';
    currentEmail = status.email;

    function renderFallbackAvatar() {
        avatarEl.innerHTML = `<div class="avatar-fallback" aria-hidden="true">${initialsFrom(status.name, status.email)}</div>`;
    }

    if (status.pictureUrl) {
        avatarEl.innerHTML = '';
        const img = document.createElement('img');
        img.className = 'avatar-photo';
        img.src = status.pictureUrl;
        img.alt = '';
        img.addEventListener('error', renderFallbackAvatar);
        avatarEl.appendChild(img);
    } else {
        renderFallbackAvatar();
    }

    try {
        const quota = await GetStorageQuota();
        storageEl.hidden = false;
        if (quota.limitBytes) {
            const pct = Math.min(100, (quota.usageBytes / quota.limitBytes) * 100);
            storageEl.innerHTML = `
                <div class="storage-bar"><div class="storage-bar-fill" style="width:${pct}%"></div></div>
                <div class="storage-label">${formatBytes(quota.usageBytes)} of ${formatBytes(quota.limitBytes)}</div>
            `;
        } else {
            storageEl.innerHTML = `<div class="storage-label">${formatBytes(quota.usageBytes)} used</div>`;
        }
    } catch (err) {
        // Soft failure (spec.md FR-012): omit the storage indicator for
        // this session rather than showing an error -- name/photo above
        // are unaffected.
        console.error('Drive.GetStorageQuota failed', err);
        storageEl.hidden = true;
        storageEl.innerHTML = '';
    }
}

async function showUpload() {
    if (!shell) return;
    if (await redirectToActiveUpload(NON_TERMINAL_UPLOAD_STATUSES)) return;
    if (!shell) return; // signed out while the status check was in flight
    screenTeardown?.();
    shell.setActiveNav('upload');
    screenTeardown = renderPicker({
        container: shell.content,
        email: currentEmail,
        onUploadStarted: (uploadId: number, file: LocalFileRef) => {
            if (!shell) return; // signed out before the upload could start rendering progress
            activeUpload = { id: uploadId, fileName: file.name };
            screenTeardown?.();
            screenTeardown = renderProgress({
                container: shell.content,
                uploadId,
                fileName: file.name,
                onTerminal: () => {
                    if (activeUpload?.id === uploadId) activeUpload = null;
                },
            });
        },
        onSignedOut: () => showSignIn(),
    });
}

async function showHistory() {
    if (!shell) return;
    if (await redirectToActiveUpload(HISTORY_BLOCKING_STATUSES)) return;
    if (!shell) return; // signed out while the status check was in flight
    screenTeardown?.();
    shell.setActiveNav('history');
    screenTeardown = renderHistory({ container: shell.content });
}

// Called once per sign-in resolution (research.md §7): if a non-terminal
// upload survived a previous run, route straight to the progress screen
// instead of the picker -- the backend has already begun resuming it (or
// flagged it awaiting_confirmation) by the time Upload.GetRecoverable resolves.
async function routeAfterSignIn(_status: AuthStatus) {
    shell = mountShell();

    let recoverable: RecoverableUpload | null = null;
    try {
        recoverable = await GetRecoverable();
    } catch (err) {
        console.error('Upload.GetRecoverable failed', err);
    }

    if (!shell) return; // signed out again before this resolved

    if (recoverable) {
        const recoveredId = recoverable.id;
        activeUpload = { id: recoveredId, fileName: recoverable.fileName };
        shell.setActiveNav('upload');
        screenTeardown = renderProgress({
            container: shell.content,
            uploadId: recoverable.id,
            fileName: recoverable.fileName,
            resuming: true,
            onTerminal: () => {
                if (activeUpload?.id === recoveredId) activeUpload = null;
            },
        });
        return;
    }

    void showUpload();
}

showSignIn();
