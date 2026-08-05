// Upload-history screen: fetches Upload.ListRecent for the initial
// snapshot, then keeps rows current by subscribing to the same upload:*
// events progress.ts already consumes (contracts/wails-bindings.md),
// matching by id and prepending a row for any id not already known.
import { ListRecent, type UploadListItem } from '../api/upload';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { toPlainLanguage } from '../errors';
import { formatBytes } from '../format';

export interface HistoryScreenOptions {
    container: HTMLElement;
}

interface UploadProgressPayload {
    id: number;
    bytesSent: number;
    totalBytes: number;
}

interface UploadPausedPayload {
    id: number;
}

interface UploadAwaitingConfirmationPayload {
    id: number;
    reason: string;
}

interface UploadCompletePayload {
    id: number;
    driveFileLink: string;
}

interface UploadFailedPayload {
    id: number;
    reason: string;
}

const FILE_TYPE_EXTENSIONS: Record<string, string> = {
    doc: '--filetype-doc',
    docx: '--filetype-doc',
    txt: '--filetype-doc',
    pdf: '--filetype-pdf',
    jpg: '--filetype-image',
    jpeg: '--filetype-image',
    png: '--filetype-image',
    gif: '--filetype-image',
    heic: '--filetype-image',
    mp3: '--filetype-audio',
    wav: '--filetype-audio',
    m4a: '--filetype-audio',
    zip: '--filetype-archive',
    rar: '--filetype-archive',
    '7z': '--filetype-archive',
};

function fileTypeToken(fileName: string): string {
    const ext = fileName.split('.').pop()?.toLowerCase() ?? '';
    return FILE_TYPE_EXTENSIONS[ext] ?? '--filetype-generic';
}

function statusPresentation(item: UploadListItem): { label: string; stateClass: string } {
    switch (item.status) {
        case 'succeeded':
            return { label: 'Succeeded', stateClass: 'state-success' };
        case 'failed':
            return {
                label: item.failureReason ? `Failed: ${toPlainLanguage(item.failureReason)}` : 'Failed',
                stateClass: 'state-error',
            };
        case 'paused':
        case 'awaiting_confirmation':
            return { label: 'Needs attention', stateClass: 'state-warning' };
        case 'in_progress':
            return {
                label:
                    item.totalBytes > 0
                        ? `${formatBytes(item.bytesSent)} of ${formatBytes(item.totalBytes)}`
                        : formatBytes(item.bytesSent),
                stateClass: 'state-loading',
            };
        case 'pending':
            return { label: 'Pending…', stateClass: 'state-loading' };
        case 'cancelled':
        default:
            return { label: 'Cancelled', stateClass: '' };
    }
}

export function renderHistory(opts: HistoryScreenOptions): () => void {
    const { container } = opts;

    let disposed = false;
    const items = new Map<number, UploadListItem>();
    let order: number[] = []; // most-recent-first display order

    container.innerHTML = `
        <div class="history-screen">
            <h1>Upload history</h1>
            <p id="history-loading" class="state-loading" role="status" aria-live="polite">
                <span class="spinner" aria-hidden="true"></span>Loading uploads…
            </p>
            <p id="history-error" class="state-error" role="alert" hidden></p>
            <p id="history-empty" class="history-empty" hidden>No uploads yet.</p>
            <ul id="history-list" class="history-list"></ul>
        </div>
    `;

    const loadingEl = container.querySelector<HTMLParagraphElement>('#history-loading')!;
    const errorEl = container.querySelector<HTMLParagraphElement>('#history-error')!;
    const emptyEl = container.querySelector<HTMLParagraphElement>('#history-empty')!;
    const listEl = container.querySelector<HTMLUListElement>('#history-list')!;

    function renderRow(item: UploadListItem): HTMLLIElement {
        const { label, stateClass } = statusPresentation(item);
        const li = document.createElement('li');
        li.className = 'history-row';
        li.dataset.id = String(item.id);
        li.innerHTML = `
            <span class="history-row-icon" style="background: var(${fileTypeToken(item.fileName)})" aria-hidden="true"></span>
            <div class="history-row-main">
                <div class="history-row-name">${item.fileName}</div>
                <div class="history-row-folder">${item.driveFolderName}</div>
            </div>
            <div class="history-row-status ${stateClass}">${label}</div>
        `;
        // Only a succeeded row with a Drive link is actually actionable --
        // a focusable row with nothing to activate is a keyboard-nav dead
        // end, so tabIndex/the click handler are added only here (FR-007).
        if (item.status === 'succeeded' && item.driveFileLink) {
            const link = item.driveFileLink;
            li.tabIndex = 0;
            li.setAttribute('role', 'link');
            li.setAttribute('aria-label', `Open ${item.fileName} in Drive`);
            const open = () => window.open(link, '_blank', 'noopener,noreferrer');
            li.addEventListener('click', open);
            li.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    open();
                }
            });
        }
        return li;
    }

    function renderList() {
        listEl.innerHTML = '';
        emptyEl.hidden = order.length !== 0;
        for (const id of order) {
            const item = items.get(id);
            if (item) listEl.appendChild(renderRow(item));
        }
    }

    function getOrCreateItem(id: number): UploadListItem {
        let item = items.get(id);
        if (!item) {
            // An event for an id outside the initial snapshot -- prepend a
            // placeholder row rather than dropping the update
            // (contracts/wails-bindings.md's Emitted events section).
            item = {
                id,
                fileName: `Upload #${id}`,
                driveFolderName: 'My Drive',
                status: 'in_progress',
                bytesSent: 0,
                totalBytes: 0,
                startedAt: new Date().toISOString(),
            };
            items.set(id, item);
            order = [id, ...order];
        }
        return item;
    }

    function applyUpdate(id: number, patch: Partial<UploadListItem>) {
        if (disposed) return;
        const item = getOrCreateItem(id);
        Object.assign(item, patch);
        renderList();
    }

    const unsubProgress = EventsOn('upload:progress', (payload: UploadProgressPayload) => {
        applyUpdate(payload.id, {
            status: 'in_progress',
            bytesSent: payload.bytesSent,
            totalBytes: payload.totalBytes,
        });
    });
    const unsubPaused = EventsOn('upload:paused', (payload: UploadPausedPayload) => {
        applyUpdate(payload.id, { status: 'paused' });
    });
    const unsubAwaitingConfirmation = EventsOn(
        'upload:awaiting-confirmation',
        (payload: UploadAwaitingConfirmationPayload) => {
            applyUpdate(payload.id, { status: 'awaiting_confirmation' });
        },
    );
    const unsubComplete = EventsOn('upload:complete', (payload: UploadCompletePayload) => {
        applyUpdate(payload.id, { status: 'succeeded', driveFileLink: payload.driveFileLink });
    });
    const unsubFailed = EventsOn('upload:failed', (payload: UploadFailedPayload) => {
        applyUpdate(payload.id, { status: 'failed', failureReason: payload.reason });
    });

    void ListRecent()
        .then((list) => {
            if (disposed) return;
            for (const item of list) {
                items.set(item.id, item);
            }
            order = list.map((item) => item.id);
            loadingEl.hidden = true;
            renderList();
        })
        .catch((err) => {
            if (disposed) return;
            loadingEl.hidden = true;
            const message = err instanceof Error ? err.message : String(err);
            errorEl.hidden = false;
            errorEl.textContent = toPlainLanguage(message);
        });

    return () => {
        disposed = true;
        unsubProgress();
        unsubPaused();
        unsubAwaitingConfirmation();
        unsubComplete();
        unsubFailed();
    };
}
