import './style.css';
import './app.css';

import { renderSignIn } from './screens/signin';
import { renderPicker } from './screens/picker';
import { renderProgress } from './screens/progress';
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
    });
}

showSignIn();
