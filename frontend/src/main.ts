import './style.css';
import './app.css';

import { renderSignIn } from './screens/signin';
import type { AuthStatus } from './api/auth';

const app = document.querySelector<HTMLElement>('#app')!;

let teardown: (() => void) | null = null;

function showSignIn() {
    teardown?.();
    teardown = renderSignIn({
        container: app,
        onSignedIn: (status: AuthStatus) => showSignedIn(status),
    });
}

function showSignedIn(status: AuthStatus) {
    teardown?.();
    teardown = null;
    // The picker screen (User Story 2) and progress screen (User Story 3)
    // replace this placeholder in later phases of this feature.
    app.innerHTML = `
        <div class="picker-screen">
            <h1>Ballast</h1>
            <p>Signed in as ${status.email ?? 'unknown'}.</p>
        </div>
    `;
}

showSignIn();
