// App shell: title bar, sidebar nav, status bar, view router and toast —
// ported from the Tether prototype's AppShell.tsx, wired to the real
// Wails/Drive backend via ./live.ts. Journal and Field Lab (routed to below)
// still run on the static mock data in ./mock-data.ts -- there's no real
// per-chunk event log or "break it on purpose" endpoint to back them.

import { icon, button, avatar } from './components';
import { formatBytes, formatDuration, formatSpeed } from './format';
import * as live from './live';
import { WindowMinimise, WindowToggleMaximise, Quit, Environment } from '../../wailsjs/runtime/runtime';
import { toPlainLanguage } from '../errors';
import { renderHome } from './views/home';
import { renderTransfers } from './views/transfers';
import { renderJournal } from './views/journal';
import { renderLab } from './views/lab';
import { renderSettings } from './views/settings';
import { renderOnboarding } from './views/onboarding';
import { openFilePicker } from './views/filepicker';

export type ViewId = 'home' | 'transfers' | 'journal' | 'lab' | 'settings';

export interface ViewCtx {
    goto: (view: ViewId) => void;
    openPicker: () => void;
    showToast: (msg: string) => void;
    selectedUploadId: string | null;
    select: (id: string | null) => void;
    filter: 'active' | 'all' | 'secured';
    setFilter: (f: 'active' | 'all' | 'secured') => void;
    rerender: () => void;
    signOut: () => void;
}

const NAV: { id: ViewId; label: string; iconFn: () => string }[] = [
    { id: 'home', label: 'Home', iconFn: () => icon.home() },
    { id: 'transfers', label: 'Transfers', iconFn: () => icon.layers() },
    { id: 'journal', label: 'Journal', iconFn: () => icon.pulse() },
    { id: 'lab', label: 'Field lab', iconFn: () => icon.flask() },
    { id: 'settings', label: 'Settings', iconFn: () => icon.gear() },
];

interface State {
    view: ViewId;
    selectedUploadId: string | null;
    filter: 'active' | 'all' | 'secured';
    toast: string | null;
    platform: 'darwin' | 'windows' | 'linux';
    // Whether onboarding should be showing. Deliberately NOT derived from
    // live.profile.signedIn on every render -- onboarding's own sign-in
    // step flips that flag true mid-flow, and re-deriving here would kick
    // the user straight to the main shell before the destination/ready
    // steps get a chance to show. Only syncAuthGate() (boot + external
    // sign-out) and onboarding's onFinish are allowed to change it.
    showOnboarding: boolean;
}

// Wails only injects `window.runtime` inside the compiled desktop app; guard
// every call so `npm run dev` (a plain browser tab used for design preview)
// doesn't throw when a window control is clicked.
function callRuntime(fn: () => void): void {
    if (typeof window !== 'undefined' && (window as any).runtime) {
        try {
            fn();
        } catch {
            /* not running inside the Wails webview */
        }
    }
}

export function mountApp(root: HTMLElement): void {
    const state: State = {
        view: 'home',
        selectedUploadId: null,
        filter: 'active',
        toast: null,
        platform: 'darwin',
        showOnboarding: true,
    };
    let toastTimer: ReturnType<typeof setTimeout> | null = null;
    let bootChecked = false;
    let lastSignedIn = false;
    let onboardingMounted = false;

    // Decides showOnboarding on the initial AuthGetStatus resolution, and
    // on a later external sign-out (revoked/expired session, or the
    // Settings sign-out button) -- never on onboarding's own in-flight
    // sign-in, which flips live.profile.signedIn well before onFinish.
    function syncAuthGate() {
        if (!bootChecked) {
            bootChecked = true;
            state.showOnboarding = !live.profile.signedIn;
        } else if (lastSignedIn && !live.profile.signedIn) {
            state.showOnboarding = true;
        }
        lastSignedIn = live.profile.signedIn;
    }

    if (typeof window !== 'undefined' && (window as any).runtime) {
        Environment()
            .then((env) => {
                if (env.platform === 'windows' || env.platform === 'linux') {
                    state.platform = env.platform;
                    render();
                }
            })
            .catch(() => {});
    }

    function showToast(msg: string) {
        state.toast = msg;
        render();
        if (toastTimer) clearTimeout(toastTimer);
        toastTimer = setTimeout(() => {
            state.toast = null;
            render();
        }, 3600);
    }

    const ctx: ViewCtx = {
        goto: (view) => {
            state.view = view;
            if (view !== 'transfers') state.selectedUploadId = null;
            render();
        },
        openPicker: () => {
            if (live.hasActiveUpload()) {
                showToast('Finish or cancel the current upload before starting another.');
                return;
            }
            openFilePicker({
                onClose: render,
                onStarted: () => {
                    state.view = 'transfers';
                    render();
                    showToast('Upload started.');
                },
            });
        },
        showToast,
        get selectedUploadId() {
            return state.selectedUploadId;
        },
        select: (id) => {
            state.selectedUploadId = id;
            render();
        },
        get filter() {
            return state.filter;
        },
        setFilter: (f) => {
            state.filter = f;
            render();
        },
        rerender: render,
        signOut: () => {
            void live.signOut().then((result) => {
                if (result.error) showToast(toPlainLanguage(result.error));
                // Local state clears via the auth:changed event, which
                // triggers render() through live.subscribe below.
            });
        },
    };

    function render() {
        syncAuthGate();
        if (state.showOnboarding) {
            // Mount onboarding exactly once per onboarding session rather
            // than on every render() call -- onboarding owns its own
            // step/folder/etc. state across an async sign-in, and a live
            // event firing mid-flow (the sign-in itself included) would
            // otherwise re-invoke renderOnboarding and reset it to step 0.
            if (!onboardingMounted) {
                onboardingMounted = true;
                root.innerHTML = `<div id="onboarding-root"></div>`;
                renderOnboarding(root.querySelector('#onboarding-root')!, {
                    onFinish: () => {
                        onboardingMounted = false;
                        state.showOnboarding = false;
                        state.view = 'home';
                        render();
                        showToast(`Welcome, ${(live.profile.name || live.profile.email).split(' ')[0]} — your Drive is connected.`);
                    },
                });
            }
            return;
        }
        onboardingMounted = false;

        const s = live.liveStats();
        const remaining = s.totalBytes - s.confirmedBytes;
        const globalEta = s.throughputBps > 0 ? (remaining / s.throughputBps) * 1000 : 0;
        const uploads = live.orderedUploads();
        const anyHolding = uploads.some((u) => u.status === 'reconnecting');
        const quota = live.storageQuota;

        const isMac = state.platform === 'darwin';

        root.innerHTML = `
            <div class="app-shell">
                <div class="titlebar ${isMac ? 'titlebar-mac' : 'titlebar-win'}">
                    ${
                        isMac
                            ? `<div class="mac-controls" data-drag-exempt>
                                <button type="button" class="mac-btn mac-close" id="win-close" title="Close" aria-label="Close"><span>${icon.x('mac-glyph')}</span></button>
                                <button type="button" class="mac-btn mac-minimise" id="win-minimise" title="Minimize" aria-label="Minimize"><span class="mac-glyph-dash"></span></button>
                                <button type="button" class="mac-btn mac-maximise" id="win-maximise" title="Maximize" aria-label="Maximize"><span class="mac-glyph-diamond"></span></button>
                            </div>`
                            : ''
                    }
                    <div class="titlebar-center">
                        <div class="conn-pill ${anyHolding ? 'holding' : ''}" data-drag-exempt>
                            <span class="dot pulse"></span>
                            <span class="conn-pill-text">${
                                anyHolding
                                    ? 'Reconnecting — nothing lost'
                                    : s.activeCount
                                      ? `${s.activeCount} moving · ${formatSpeed(s.throughputBps)}`
                                      : 'All caught up'
                            }</span>
                            ${
                                globalEta > 0
                                    ? `<span class="conn-pill-sep">•</span><span class="conn-pill-eta tnum">${formatDuration(globalEta)} left</span>`
                                    : ''
                            }
                        </div>
                    </div>
                    <button type="button" class="account-btn" id="account-btn" data-drag-exempt>
                        ${avatar(live.profile.email || live.profile.name, live.profile.name || live.profile.email, 26)}
                        <span class="account-btn-email">${live.profile.email}</span>
                    </button>
                    ${
                        !isMac
                            ? `<div class="win-controls" data-drag-exempt>
                                <button type="button" class="win-btn" id="win-minimise" title="Minimize" aria-label="Minimize"><span class="win-glyph-dash"></span></button>
                                <button type="button" class="win-btn" id="win-maximise" title="Maximize" aria-label="Maximize"><span class="win-glyph-square"></span></button>
                                <button type="button" class="win-btn win-btn-close" id="win-close" title="Close" aria-label="Close">${icon.x('win-glyph-x')}</button>
                            </div>`
                            : ''
                    }
                </div>

                <div class="body-row">
                    <nav class="sidebar" aria-label="Main">
                        <div class="sidebar-brand-row">
                            <span class="brand-mark">${icon.bolt()}</span>
                            <div>
                                <p class="display brand-name">Ballast</p>
                                <p class="brand-tag">keeps your place</p>
                            </div>
                        </div>
                        <ul class="nav-list">
                            ${NAV.map(
                                (n) => `
                                <li>
                                    <button type="button" class="nav-item ${state.view === n.id ? 'active' : ''}" data-nav="${n.id}">
                                        ${n.iconFn()}
                                        ${n.label}
                                        ${n.id === 'transfers' && s.activeCount > 0 ? `<span class="nav-badge tnum">${s.activeCount}</span>` : ''}
                                    </button>
                                </li>`,
                            ).join('')}
                        </ul>
                        <div class="sidebar-cta">
                            ${button(`${icon.plus()} Choose files`, {
                                variant: 'primary',
                                attrs: `id="sidebar-pick" ${live.hasActiveUpload() ? 'disabled title="Finish or cancel the current upload first"' : ''}`,
                            })}
                        </div>
                        <div class="sidebar-bottom">
                            <div class="mini-card">
                                <p class="mini-card-title">${icon.drive('icon fg-grape')} Drive storage</p>
                                ${
                                    quota
                                        ? `<p class="mini-card-sub tnum">${formatBytes(quota.usageBytes)} of ${quota.limitBytes ? formatBytes(quota.limitBytes) : 'unlimited'}</p>
                                           ${
                                               quota.limitBytes
                                                   ? `<div class="storage-bar-track"><div class="storage-bar-fill2" style="width:${Math.min(100, (quota.usageBytes / quota.limitBytes) * 100)}%"></div></div>`
                                                   : ''
                                           }`
                                        : `<p class="mini-card-sub">Fetching usage…</p>`
                                }
                            </div>
                            <div class="mini-card autoresume-card on">
                                <p class="mini-card-title">${icon.shield()} Auto-resume is on</p>
                                <p class="mini-card-sub">Reopens dropped transfers automatically, from the exact byte Drive confirmed.</p>
                            </div>
                        </div>
                    </nav>

                    <main class="content-area">
                        <section class="view-pane" id="shell-content"></section>
                    </main>
                </div>

                <div class="statusbar">
                    <div class="statusbar-left">
                        <span style="display:flex;align-items:center;gap:6px"><span class="statusbar-dot"></span>Connected</span>
                        <span>chunk receipts written before we tell you</span>
                    </div>
                    <div class="statusbar-right tnum">
                        <span>${uploads.length} session${uploads.length === 1 ? '' : 's'}</span>
                        <span style="color:var(--color-grass)">journal healthy</span>
                    </div>
                </div>

                ${
                    state.toast
                        ? `<div class="toast rise"><span class="toast-icon">${icon.check()}</span><span>${state.toast}</span></div>`
                        : ''
                }
            </div>
        `;

        root.querySelector('#account-btn')?.addEventListener('click', () => ctx.goto('settings'));
        root.querySelector('#sidebar-pick')?.addEventListener('click', () => ctx.openPicker());
        root.querySelectorAll<HTMLButtonElement>('[data-nav]').forEach((el) => {
            el.addEventListener('click', () => ctx.goto(el.dataset.nav as ViewId));
        });
        root.querySelector('#win-close')?.addEventListener('click', () => callRuntime(() => Quit()));
        root.querySelector('#win-minimise')?.addEventListener('click', () => callRuntime(() => WindowMinimise()));
        root.querySelector('#win-maximise')?.addEventListener('click', () => callRuntime(() => WindowToggleMaximise()));

        const content = root.querySelector<HTMLElement>('#shell-content')!;
        if (state.view === 'home') renderHome(content, ctx);
        else if (state.view === 'transfers') renderTransfers(content, ctx);
        else if (state.view === 'journal') renderJournal(content, ctx);
        else if (state.view === 'lab') renderLab(content, ctx);
        else renderSettings(content, ctx);
    }

    root.innerHTML = `<div class="app-shell" style="align-items:center;justify-content:center;display:flex"></div>`;
    live.subscribe(render);
    live.initLive(render);
}
