// Upload progress/result screen: listens for upload:progress,
// upload:complete, and upload:failed, and reconciles with Upload.GetStatus
// on mount so the UI reflects the true state even after a reload.
import { GetStatus, type UploadStatus } from '../api/upload';
import { EventsOn } from '../../wailsjs/runtime/runtime';

export interface ProgressScreenOptions {
    container: HTMLElement;
    uploadId: number;
    fileName: string;
}

interface UploadProgressPayload {
    id: number;
    bytesSent: number;
    totalBytes: number;
}

interface UploadCompletePayload {
    id: number;
    driveFileLink: string;
}

interface UploadFailedPayload {
    id: number;
    reason: string;
}

export function renderProgress(opts: ProgressScreenOptions): () => void {
    const { container, uploadId, fileName } = opts;

    container.innerHTML = `
        <div class="progress-screen">
            <h1>Ballast</h1>
            <p class="progress-file">Uploading ${fileName}… (upload #${uploadId})</p>
            <p id="progress-bytes" class="progress-bytes"></p>
            <p id="progress-result" class="progress-result" role="status" aria-live="polite"></p>
        </div>
    `;

    const bytesEl = container.querySelector<HTMLParagraphElement>('#progress-bytes')!;
    const resultEl = container.querySelector<HTMLParagraphElement>('#progress-result')!;

    function renderBytes(bytesSent: number, totalBytes: number) {
        bytesEl.textContent = totalBytes > 0 ? `${bytesSent} / ${totalBytes} bytes` : `${bytesSent} bytes`;
    }

    function renderSuccess(driveFileLink: string) {
        resultEl.classList.remove('progress-result--failed');
        resultEl.classList.add('progress-result--success');
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
        resultEl.classList.remove('progress-result--success');
        resultEl.classList.add('progress-result--failed');
        resultEl.textContent = `Upload failed: ${reason}`;
    }

    const unsubProgress = EventsOn('upload:progress', (payload: UploadProgressPayload) => {
        if (payload.id !== uploadId) return;
        renderBytes(payload.bytesSent, payload.totalBytes);
    });
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
            }
        })
        .catch(() => {
            // Non-fatal -- live events still drive the UI from here.
        });

    return () => {
        unsubProgress();
        unsubComplete();
        unsubFailed();
    };
}
