// Sign-in screen: calls Auth.GetStatus on load, lets the user trigger
// Auth.SignIn, and reacts to auth:changed so a sign-out or forced session
// clear always brings the UI back here.
import { GetStatus, SignIn, type AuthStatus } from '../api/auth';
import { EventsOn } from '../../wailsjs/runtime/runtime';

export interface SignInScreenOptions {
    container: HTMLElement;
    onSignedIn: (status: AuthStatus) => void;
}

// Substring used to recognize the keychain-unavailable error so it can be
// surfaced distinctly from a generic sign-in failure.
const KEYCHAIN_UNAVAILABLE_MARKER = 'secure credential storage';

export function renderSignIn(opts: SignInScreenOptions): () => void {
    const { container, onSignedIn } = opts;

    container.innerHTML = `
        <div class="signin-screen">
            <h1>Ballast</h1>
            <p>Connect your Google account to upload files to Google Drive.</p>
            <button id="signin-btn" class="btn">Sign in with Google</button>
            <p id="signin-status" class="signin-status" role="status" aria-live="polite"></p>
            <p id="signin-error" class="signin-error" role="alert" aria-live="assertive"></p>
        </div>
    `;

    const button = container.querySelector<HTMLButtonElement>('#signin-btn')!;
    const statusEl = container.querySelector<HTMLParagraphElement>('#signin-status')!;
    const errorEl = container.querySelector<HTMLParagraphElement>('#signin-error')!;

    function setBusy(busy: boolean) {
        button.disabled = busy;
        statusEl.textContent = busy ? 'Waiting for you to finish signing in with Google…' : '';
    }

    function showError(message: string) {
        errorEl.textContent = message;
        errorEl.classList.toggle(
            'signin-error--keychain',
            message.toLowerCase().includes(KEYCHAIN_UNAVAILABLE_MARKER),
        );
    }

    function clearError() {
        errorEl.textContent = '';
        errorEl.classList.remove('signin-error--keychain');
    }

    async function checkExistingSession() {
        try {
            const status = await GetStatus();
            if (status.signedIn) {
                onSignedIn(status);
            }
        } catch (err) {
            // Leave the sign-in button available rather than getting stuck.
            console.error('Auth.GetStatus failed', err);
        }
    }

    button.addEventListener('click', async () => {
        clearError();
        setBusy(true);
        try {
            const status = await SignIn();
            setBusy(false);
            if (status.signedIn) {
                onSignedIn(status);
            }
            // signedIn === false with no error means the user cancelled/denied consent; no error shown.
        } catch (err) {
            setBusy(false);
            const message = err instanceof Error ? err.message : String(err);
            showError(message);
        }
    });

    const unsubscribe = EventsOn('auth:changed', (status: AuthStatus) => {
        if (status.signedIn) {
            onSignedIn(status);
        }
    });

    void checkExistingSession();

    return unsubscribe;
}
