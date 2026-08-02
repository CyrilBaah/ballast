import './style.css';
import './app.css';

import { renderSignIn } from './screens/signin';
import { renderPicker } from './screens/picker';
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
            // The upload progress/result screen (User Story 3) replaces
            // this placeholder in a later phase of this feature.
            teardown?.();
            teardown = null;
            app.innerHTML = `
                <div class="progress-screen">
                    <h1>Ballast</h1>
                    <p>Uploading ${file.name}… (upload #${uploadId})</p>
                </div>
            `;
        },
    });
}

showSignIn();
