// Onboarding — ported from Onboarding.tsx, now wired to the real backend:
// step 1 triggers a real Google sign-in (the system browser handles account
// choice, so there's no in-app account-picker step anymore), and step 2
// browses real Drive folders.

import { icon, button, ring } from '../components';
import * as live from '../live';
import type { DriveFolder } from '../live';
import { toPlainLanguage } from '../../errors';

const STEPS = ['Welcome', 'Sign in', 'Destination', 'Ready'];

export function renderOnboarding(root: HTMLElement, opts: { onFinish: () => void }): void {
    let step = 0;
    let signingIn = false;
    let signInError = '';
    let folder: { id: string; name: string } = { id: '', name: 'My Drive' };
    let folders: DriveFolder[] = [];
    let loadingFolders = false;
    let folderError = '';

    function render() {
        root.innerHTML = `
            <div class="mesh onboarding-shell">
                <div class="grain" style="position:absolute;inset:0"></div>
                <div class="floaty onboarding-blob onboarding-blob-a"></div>
                <div class="floaty onboarding-blob onboarding-blob-b"></div>

                <div class="onboarding-center">
                    <div class="onboarding-card">
                        <div class="mesh-photo onboarding-story">
                            <div class="onboarding-story-top">
                                <span class="onboarding-story-mark">${icon.bolt()}</span>
                                <span class="onboarding-story-brand">Ballast</span>
                            </div>
                            <div class="onboarding-story-bottom">
                                <p class="display onboarding-story-title">Your footage gets home.<br/>Even when the signal doesn't.</p>
                                <p class="onboarding-story-sub">Ballast remembers every byte Google Drive has already accepted. Lose the link, crash the laptop, cross a border — it picks up exactly where it stopped.</p>
                                <div class="onboarding-story-tags">
                                    ${['No re-uploading', 'Survives reboots', 'Verified on arrival'].map((t) => `<span class="onboarding-tag">${t}</span>`).join('')}
                                </div>
                            </div>
                        </div>

                        <div class="onboarding-steps">
                            <div class="onboarding-progress">
                                ${STEPS.map((_, i) => `<span class="onboarding-progress-dot ${i === step ? 'current' : i < step ? 'done' : ''}"></span>`).join('')}
                                <span class="onboarding-progress-label">Step ${step + 1} of ${STEPS.length}</span>
                            </div>

                            <div class="onboarding-step-body" id="onboarding-step-body">
                                ${stepMarkup(step)}
                            </div>

                            <div class="onboarding-footer">
                                <button type="button" class="onboarding-back ${step === 0 || step === 1 ? 'invisible' : ''}" id="onboarding-back">Back</button>
                                <span id="onboarding-next-slot"></span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `;

        root.querySelector('#onboarding-back')?.addEventListener('click', () => {
            step = Math.max(0, step - 1);
            render();
        });

        const nextSlot = root.querySelector('#onboarding-next-slot')!;
        if (step === 0) {
            nextSlot.innerHTML = button(`Get started ${icon.arrow()}`, { variant: 'primary', size: 'lg' });
            nextSlot.querySelector('button')?.addEventListener('click', () => {
                step = 1;
                render();
            });
        } else if (step === 2) {
            nextSlot.innerHTML = button(`Continue ${icon.arrow()}`, { variant: 'primary', size: 'lg' });
            nextSlot.querySelector('button')?.addEventListener('click', () => {
                step = 3;
                render();
            });
        } else if (step === 3) {
            nextSlot.innerHTML = button(`Open Ballast ${icon.arrow()}`, { variant: 'primary', size: 'lg' });
            nextSlot.querySelector('button')?.addEventListener('click', () => {
                live.setDefaultFolder(folder);
                opts.onFinish();
            });
        }

        if (step === 1) wireSignInStep();
        if (step === 2) wireDestinationStep();
    }

    function wireSignInStep() {
        const body = root.querySelector<HTMLElement>('#onboarding-step-body')!;
        body.querySelector('#continue-google')?.addEventListener('click', () => {
            if (signingIn) return;
            signingIn = true;
            signInError = '';
            render();
            void live.signIn().then((result) => {
                signingIn = false;
                if (result.ok) {
                    step = 2;
                    render();
                    void loadFolders();
                } else {
                    // signedIn:false with no error means the user
                    // cancelled/denied consent in the browser -- no error to show.
                    if (result.error) signInError = toPlainLanguage(result.error);
                    render();
                }
            });
        });
    }

    async function loadFolders() {
        loadingFolders = true;
        folderError = '';
        render();
        try {
            folders = await live.listDriveFolders('');
        } catch (err) {
            folders = [];
            folderError = toPlainLanguage(err instanceof Error ? err.message : String(err));
        } finally {
            loadingFolders = false;
        }
        render();
    }

    function wireDestinationStep() {
        const body = root.querySelector<HTMLElement>('#onboarding-step-body')!;
        body.querySelectorAll<HTMLElement>('[data-folder]').forEach((el) => {
            el.addEventListener('click', () => {
                folder = { id: el.dataset.folder!, name: el.dataset.folderName! };
                step = 3;
                render();
            });
        });
    }

    function stepMarkup(step: number): string {
        if (step === 0) {
            return `<div class="rise">
                <span class="pill-badge tone-tangerine">${icon.sparkle('icon')} Welcome aboard</span>
                <h1 class="display onboarding-h1">Big files.<br/>Bad internet.<br/><span class="fg-grass">No panic.</span></h1>
                <p class="onboarding-p">Set up takes about thirty seconds. After that you can close the lid, lose the tower, or run out of battery — nothing you've already sent is ever sent twice.</p>
                <div class="onboarding-features">
                    ${feature('grass', icon.shield('icon'), 'Chunk-level receipts', 'Every piece Drive accepts is written to disk before we move on.')}
                    ${feature('sky', icon.refresh('icon'), 'Resumes by itself', "Reconnects with backoff the moment there's a signal again.")}
                    ${feature('grape', icon.check('icon'), 'Proof it arrived', 'Verified against what Drive reports before we call it done.')}
                </div>
            </div>`;
        }
        if (step === 1) {
            return `<div class="rise">
                <h1 class="display onboarding-h2">Connect your Google Drive</h1>
                <p class="onboarding-p">Ballast only asks for the <span style="font-weight:500;color:var(--color-ink)">drive.file</span> scope — it can see the files it creates for you, nothing else in your Drive.</p>
                <div style="margin-top:24px">
                    <button type="button" class="lift onboarding-google-btn" id="continue-google" ${signingIn ? 'disabled' : ''}>
                        ${icon.google('icon icon-lg')}${signingIn ? 'Waiting for you to finish in the browser…' : 'Continue with Google'}
                    </button>
                    <p style="margin-top:12px;text-align:center;font-size:11.5px;color:var(--color-faint)">We never store your password. Tokens live in your OS keychain.</p>
                    ${signInError ? `<p class="state-error" style="margin-top:12px;text-align:center;font-size:12px">${signInError}</p>` : ''}
                </div>
            </div>`;
        }
        if (step === 2) {
            return `<div class="rise">
                <h1 class="display onboarding-h2">Where should things land?</h1>
                <p class="onboarding-p">Pick a home folder in Drive. You can override it for any individual upload later.</p>
                <div class="onboarding-folder-list">
                    ${
                        loadingFolders
                            ? `<p class="state-loading" style="font-size:12.5px;color:var(--color-mute)"><span class="spinner" aria-hidden="true"></span>Loading your Drive folders…</p>`
                            : folderError
                              ? `<p class="state-error" style="font-size:12.5px">${folderError}</p>`
                              : destinationRows()
                    }
                </div>
            </div>`;
        }
        return `<div class="rise">
            <div style="display:flex;align-items:center;gap:12px">
                ${ring(1, { size: 54, stroke: 5, inner: icon.check('icon fg-grass') })}
                <div>
                    <h1 class="display onboarding-h3">You're all set, ${(live.profile.name || live.profile.email).split(' ')[0]}.</h1>
                    <p style="margin:0;font-size:12.5px;color:var(--color-mute)">Uploading into <span style="font-weight:500;color:var(--color-ink)">${folder.name}</span></p>
                </div>
            </div>

            <div class="onboarding-summary-grid">
                ${summary('Signed in as', live.profile.email)}
                ${summary('Auto-resume', 'On · exponential backoff')}
                ${summary('Default folder', folder.name)}
            </div>
        </div>`;
    }

    function destinationRows(): string {
        const rows = [{ id: '', name: 'My Drive', hasChildren: false } as DriveFolder, ...folders];
        return rows
            .map((d) => {
                const active = folder.id === d.id;
                return `
                <button type="button" class="onboarding-folder-row ${active ? 'active' : ''}" data-folder="${d.id}" data-folder-name="${d.name}">
                    <span class="onboarding-folder-icon tone-grass">${icon.folder('icon icon-lg')}</span>
                    <span style="min-width:0;flex:1">
                        <span style="display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13px;font-weight:500;color:var(--color-ink)">${d.name}</span>
                    </span>
                    <span class="onboarding-folder-check ${active ? 'active' : ''}">${active ? icon.check('icon') : ''}</span>
                </button>`;
            })
            .join('');
    }

    render();
}

function feature(tone: string, iconSvg: string, title: string, sub: string): string {
    return `<div class="onboarding-feature">
        <span class="onboarding-feature-icon tone-${tone}">${iconSvg}</span>
        <span>
            <span style="display:block;font-size:13px;font-weight:500;color:var(--color-ink)">${title}</span>
            <span style="display:block;font-size:12px;line-height:1.5;color:var(--color-mute)">${sub}</span>
        </span>
    </div>`;
}

function summary(label: string, value: string): string {
    return `<div class="onboarding-summary-tile">
        <p style="margin:0;font-size:10.5px;font-weight:500;text-transform:uppercase;letter-spacing:0.1em;color:var(--color-faint)">${label}</p>
        <p style="margin:4px 0 0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12.5px;color:var(--color-ink)">${value}</p>
    </div>`;
}
