// Upload progress/result screen: listens for upload:progress,
// upload:paused, upload:awaiting-confirmation, upload:complete, and
// upload:failed, and reconciles with Upload.GetStatus on mount so the UI
// reflects the true state even after a reload.
import { GetStatus, ConfirmRestart, Cancel, type UploadStatus } from '../api/upload';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { toPlainLanguage } from '../errors';

export interface ProgressScreenOptions {
    container: HTMLElement;
    uploadId: number;
    fileName: string;
    // True when this screen is showing an upload recovered from a
    // previous run (Upload.GetRecoverable) rather than one just started --
    // shows "Resuming upload…" until the first live event/status arrives.
    resuming?: boolean;
}

interface UploadProgressPayload {
    id: number;
    bytesSent: number;
    totalBytes: number;
}

interface UploadPausedPayload {
    id: number;
    retryingSince: string;
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

const CONFIRMATION_COPY: Record<string, string> = {
    session_expired: "This upload's session expired. Restart from the beginning?",
    file_changed: 'The local file changed since this upload started. Restart from the beginning?',
};

export function renderProgress(opts: ProgressScreenOptions): () => void {
    const { container, uploadId, fileName, resuming } = opts;

    container.innerHTML = `
        <div class="progress-screen">
            <h1>Ballast</h1>
            <p class="progress-file">Uploading ${fileName}… (upload #${uploadId})</p>
            <p id="progress-bytes" class="progress-bytes"></p>
            <p id="progress-result" class="progress-result" role="status" aria-live="polite">${
                resuming ? 'Resuming upload…' : ''
            }</p>
            <div id="progress-confirm" class="progress-confirm" hidden>
                <p id="progress-confirm-text" role="alert"></p>
                <button id="progress-confirm-restart-btn" class="btn">Restart from the beginning</button>
                <button id="progress-confirm-cancel-btn" class="btn btn-secondary">Cancel upload</button>
            </div>
        </div>
    `;

    const bytesEl = container.querySelector<HTMLParagraphElement>('#progress-bytes')!;
    const resultEl = container.querySelector<HTMLParagraphElement>('#progress-result')!;
    const confirmEl = container.querySelector<HTMLDivElement>('#progress-confirm')!;
    const confirmTextEl = container.querySelector<HTMLParagraphElement>('#progress-confirm-text')!;
    const confirmRestartBtn = container.querySelector<HTMLButtonElement>('#progress-confirm-restart-btn')!;
    const confirmCancelBtn = container.querySelector<HTMLButtonElement>('#progress-confirm-cancel-btn')!;

    function renderBytes(bytesSent: number, totalBytes: number) {
        bytesEl.textContent = totalBytes > 0 ? `${bytesSent} / ${totalBytes} bytes` : `${bytesSent} bytes`;
    }

    function hideConfirmation() {
        confirmEl.hidden = true;
    }

    const RESULT_STATE_CLASSES = ['state-success', 'state-error', 'state-warning', 'state-loading'];

    function setResultState(stateClass: string | null) {
        resultEl.classList.remove(...RESULT_STATE_CLASSES);
        if (stateClass) resultEl.classList.add(stateClass);
    }

    function renderRetrying() {
        hideConfirmation();
        setResultState('state-warning');
        resultEl.textContent = 'Still in progress — retrying…';
    }

    function renderInProgress() {
        hideConfirmation();
        setResultState('state-loading');
        resultEl.textContent = '';
    }

    function renderSuccess(driveFileLink: string) {
        hideConfirmation();
        setResultState('state-success');
        resultEl.innerHTML = '';
        resultEl.appendChild(document.createTextNode('Upload complete — '));
        const link = document.createElement('a');
        link.href = driveFileLink;
        link.target = '_blank';
        link.rel = 'noopener noreferrer';
        link.textContent = 'view in Drive';
        resultEl.appendChild(link);
    }

    function renderFailure(reason: string) {
        hideConfirmation();
        setResultState('state-error');
        resultEl.textContent = `Upload failed: ${toPlainLanguage(reason)}`;
    }

    function renderCancelled() {
        hideConfirmation();
        setResultState(null);
        resultEl.textContent = 'Upload cancelled.';
    }

    function renderAwaitingConfirmation(reason: string) {
        setResultState('state-warning');
        resultEl.textContent = '';
        confirmTextEl.textContent = CONFIRMATION_COPY[reason] ?? 'Restart this upload from the beginning?';
        confirmEl.hidden = false;
        confirmRestartBtn.disabled = false;
        confirmCancelBtn.disabled = false;
    }

    confirmRestartBtn.addEventListener('click', async () => {
        confirmRestartBtn.disabled = true;
        confirmCancelBtn.disabled = true;
        try {
            await ConfirmRestart(uploadId);
            hideConfirmation();
            renderInProgress();
        } catch (err) {
            confirmRestartBtn.disabled = false;
            confirmCancelBtn.disabled = false;
            const message = err instanceof Error ? err.message : String(err);
            confirmTextEl.textContent = toPlainLanguage(message);
        }
    });

    confirmCancelBtn.addEventListener('click', async () => {
        confirmRestartBtn.disabled = true;
        confirmCancelBtn.disabled = true;
        try {
            await Cancel(uploadId);
            renderCancelled();
        } catch (err) {
            confirmRestartBtn.disabled = false;
            confirmCancelBtn.disabled = false;
            const message = err instanceof Error ? err.message : String(err);
            confirmTextEl.textContent = toPlainLanguage(message);
        }
    });

    const unsubProgress = EventsOn('upload:progress', (payload: UploadProgressPayload) => {
        if (payload.id !== uploadId) return;
        renderBytes(payload.bytesSent, payload.totalBytes);
        renderInProgress();
    });
    const unsubPaused = EventsOn('upload:paused', (payload: UploadPausedPayload) => {
        if (payload.id !== uploadId) return;
        renderRetrying();
    });
    const unsubAwaitingConfirmation = EventsOn(
        'upload:awaiting-confirmation',
        (payload: UploadAwaitingConfirmationPayload) => {
            if (payload.id !== uploadId) return;
            renderAwaitingConfirmation(payload.reason);
        },
    );
    const unsubComplete = EventsOn('upload:complete', (payload: UploadCompletePayload) => {
        if (payload.id !== uploadId) return;
        renderSuccess(payload.driveFileLink);
    });
    const unsubFailed = EventsOn('upload:failed', (payload: UploadFailedPayload) => {
        if (payload.id !== uploadId) return;
        renderFailure(payload.reason);
    });

    void GetStatus(uploadId)
        .then((status: UploadStatus) => {
            renderBytes(status.bytesSent, status.totalBytes);
            if (status.status === 'succeeded' && status.driveFileLink) {
                renderSuccess(status.driveFileLink);
            } else if (status.status === 'failed' && status.failureReason) {
                renderFailure(status.failureReason);
            } else if (status.status === 'paused') {
                renderRetrying();
            } else if (status.status === 'awaiting_confirmation' && status.awaitingConfirmationReason) {
                renderAwaitingConfirmation(status.awaitingConfirmationReason);
            } else if (status.status === 'cancelled') {
                renderCancelled();
            }
        })
        .catch(() => {
            // Non-fatal -- live events still drive the UI from here.
        });

    return () => {
        unsubProgress();
        unsubPaused();
        unsubAwaitingConfirmation();
        unsubComplete();
        unsubFailed();
    };
}
