// Sign-in screen: calls Auth.GetStatus on load, lets the user trigger
// Auth.SignIn, and reacts to auth:changed so a sign-out or forced session
// clear always brings the UI back here.
import { GetStatus, SignIn, type AuthStatus } from '../api/auth';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { toPlainLanguage } from '../errors';

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
            <svg class="wave-layer" viewBox="0 0 880 500" preserveAspectRatio="none" aria-hidden="true">
                <path class="w1" d="M-20,120 C120,70 220,170 360,120 C500,70 600,170 740,120 C820,95 860,110 900,100" />
                <path class="w2" d="M-20,230 C100,190 240,270 380,230 C520,190 640,270 780,230 C840,215 870,225 900,220" />
                <path class="w3" d="M-20,360 C140,320 260,400 400,360 C540,320 660,400 800,360 C850,345 875,352 900,348" />
                <circle cx="120" cy="88" r="4" fill="var(--color-accent)" />
                <circle cx="700" cy="205" r="3.5" fill="var(--color-success)" />
                <circle cx="260" cy="340" r="3" fill="var(--color-warning)" />
                <circle cx="620" cy="380" r="4" fill="var(--filetype-image)" />
            </svg>
            <div class="signin-card">
                <h1>Ballast</h1>
                <p>Connect your Google account to upload files to Google Drive.</p>
                <button id="signin-btn" class="btn">Sign in with Google</button>
                <p id="signin-status" class="signin-status" role="status" aria-live="polite"></p>
                <p id="signin-error" class="signin-error state-error" role="alert" aria-live="assertive"></p>
            </div>
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
            showError(toPlainLanguage(message));
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
