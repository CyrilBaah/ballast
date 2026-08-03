import './style.css';
import './app.css';

import { renderSignIn } from './screens/signin';
import { renderPicker } from './screens/picker';
import { renderProgress } from './screens/progress';
import { GetRecoverable, type RecoverableUpload } from './api/upload';
import type { AuthStatus } from './api/auth';
import type { LocalFileRef } from './api/files';

const app = document.querySelector<HTMLElement>('#app')!;

let teardown: (() => void) | null = null;

function showSignIn() {
    teardown?.();
    teardown = renderSignIn({
        container: app,
        onSignedIn: (status: AuthStatus) => showPicker(status),
    });
}

function showPicker(status: AuthStatus) {
    teardown?.();
    teardown = null;
    void routeAfterSignIn(status);
}

// Called once per sign-in resolution (research.md §7): if a non-terminal
// upload survived a previous run, route straight to the progress screen
// instead of the picker -- the backend has already begun resuming it (or
// flagged it awaiting_confirmation) by the time Upload.GetRecoverable resolves.
async function routeAfterSignIn(status: AuthStatus) {
    let recoverable: RecoverableUpload | null = null;
    try {
        recoverable = await GetRecoverable();
    } catch (err) {
        console.error('Upload.GetRecoverable failed', err);
    }

    if (recoverable) {
        teardown = renderProgress({
            container: app,
            uploadId: recoverable.id,
            fileName: recoverable.fileName,
            resuming: true,
        });
        return;
    }

    teardown = renderPicker({
        container: app,
        email: status.email,
        onUploadStarted: (uploadId: number, file: LocalFileRef) => {
            teardown?.();
            teardown = renderProgress({
                container: app,
                uploadId,
                fileName: file.name,
            });
        },
        onSignedOut: () => showSignIn(),
    });
}

showSignIn();
