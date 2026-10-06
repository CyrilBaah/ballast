// Settings — ported from Views.tsx's SettingsView.

import { icon, avatar, toggleHtml, sparkline, button } from '../components';
import { formatBytes } from '../format';
import * as mock from '../mock-data';
import * as live from '../live';
import type { ViewCtx } from '../shell';
import { toPlainLanguage } from '../../errors';

export function renderSettings(container: HTMLElement, ctx: ViewCtx): void {
    const s = mock.settings;
    const st = mock.stats();
    const profileName = live.profile.name || live.profile.email;
    const profileEmail = live.profile.email;

    container.innerHTML = `
        <div class="scroll view-scroll settings-scroll">
            <div class="settings-inner">
                <h1 class="display" style="margin:0;font-size:27px;line-height:1.2">Settings</h1>
                <p style="margin:8px 0 0;font-size:13.5px;line-height:1.6;color:var(--color-mute)">Defaults picked for bad connections. You can loosen them — we'd rather you didn't.</p>

                <div class="card settings-profile">
                    ${avatar(profileEmail || profileName, profileName, 52)}
                    <div style="min-width:0;flex:1">
                        <p style="margin:0;font-size:15px;font-weight:500">${profileName}</p>
                        <p style="margin:2px 0 0;font-size:12.5px;color:var(--color-mute)">${profileEmail}</p>
                        <p class="fg-grass" style="margin:6px 0 0;display:flex;align-items:center;gap:6px;font-size:11.5px">${icon.check('icon')} Google Drive connected · drive.file scope</p>
                    </div>
                    ${button('Sign out', { variant: 'outline', attrs: 'id="sign-out-btn"' })}
                </div>

                <div class="card settings-rows">
                    ${settingRow('Auto-resume', 'Reopen held sessions automatically with exponential backoff after a drop, crash or reboot.', toggleHtml(s.autoResume, 'data-toggle="autoResume"'))}
                    ${settingRow('Verify on completion', 'Compare the local MD5 against the digest Drive reports before calling a file done.', toggleHtml(s.verifyChecksums, 'data-toggle="verifyChecksums"'))}
                    ${settingRow('Default Drive folder', 'Where new uploads land unless you say otherwise.', `<span class="settings-folder-pill">${s.driveFolder}</span>`)}
                    ${settingRow('Journal location', 'Write-ahead log that survives power loss.', `<span class="mono" style="font-size:11.5px;color:var(--color-mute)">~/.ballast/journal.db</span>`)}
                </div>

                ${captionsSection()}

                <div class="card" style="margin-top:16px;padding:20px">
                    <h2 class="eyebrow">Lifetime record</h2>
                    <div class="settings-big-grid">
                        ${bigStat('Sessions recovered', String(s.sessionsRecovered), 'grape')}
                        ${bigStat('Bytes never re-sent', formatBytes(st.protectedBytes), 'grass')}
                        ${bigStat('Auto-resumes', String(st.resumeCount), 'tangerine')}
                    </div>
                    <div style="margin-top:20px;height:52px">${sparkline(s.netHistory, { color: '#3b82f6', height: 52 })}</div>
                </div>

                <p style="margin:24px 0 16px;display:flex;align-items:center;justify-content:center;gap:6px;font-size:11.5px;color:var(--color-faint)">
                    Made for people who file from far away ${icon.heart('icon fg-coral')}
                </p>
            </div>
        </div>
    `;

    container.querySelector('#sign-out-btn')?.addEventListener('click', () => ctx.signOut());
    wireCaptionSettings(container, ctx);
    container.querySelectorAll<HTMLElement>('[data-toggle]').forEach((el) => {
        el.addEventListener('click', () => {
            const key = el.dataset.toggle as 'autoResume' | 'verifyChecksums';
            mock.settings[key] = !mock.settings[key];
            ctx.rerender();
        });
    });
}

function settingRow(title: string, sub: string, control: string): string {
    return `<div class="settings-row">
        <div>
            <p style="margin:0;font-size:13.5px;font-weight:500">${title}</p>
            <p style="margin:3px 0 0;max-width:440px;font-size:12px;line-height:1.5;color:var(--color-mute)">${sub}</p>
        </div>
        ${control}
    </div>`;
}

function bigStat(label: string, value: string, tone: string): string {
    return `<div>
        <p class="tnum fg-${tone}" style="margin:0;font-size:24px;font-weight:600;letter-spacing:-0.02em">${value}</p>
        <p style="margin:2px 0 0;font-size:11.5px;color:var(--color-mute)">${label}</p>
    </div>`;
}

// Automatic captions (Feature 005, FR-009/FR-015/FR-017/FR-020).
function captionsSection(): string {
    const c = live.captionSettings;
    if (!c) {
        void live.refreshCaptionSettings();
        return `<div class="card settings-rows" data-captions-settings><p style="margin:0;padding:16px;font-size:12px;color:var(--color-mute)">Loading caption settings…</p></div>`;
    }
    const disabled = !c.available;
    const langTab = (id: 'en' | 'auto', label: string) =>
        `<button type="button" class="filter-tab ${c.language === id ? 'active' : ''}" data-caption-lang="${id}" ${disabled ? 'disabled' : ''}>${label}</button>`;
    return `<div class="card settings-rows" data-captions-settings style="margin-top:16px">
        ${settingRow(
            'Automatic captions',
            'Make a caption file for every video you upload, on this Mac. Saved next to the video in Drive and on your Mac.',
            `<span ${disabled ? 'style="opacity:0.45;pointer-events:none"' : ''}>${toggleHtml(c.enabled && !disabled, 'data-captions-toggle')}</span>`,
        )}
        ${settingRow('Caption language', 'English works best for most videos. Automatic detects the language for each part of the video.', `<div class="filter-tabs">${langTab('en', 'English')}${langTab('auto', 'Automatic')}</div>`)}
        <p data-captions-notice style="margin:0;padding:12px 20px 16px;font-size:11.5px;line-height:1.6;color:var(--color-mute)">${
            disabled
                ? `<span class="fg-coral">${c.unavailableReason ?? "Captions aren't available on this system yet"}</span>`
                : `Captions are made on this computer, so nothing but the finished caption file leaves your Mac. Accuracy may be lower for languages other than English, including Twi and Ga. Captions are saved as a separate .srt file next to the video — in Drive, open the video, then ⋮ › Manage caption tracks to attach it.${c.modelDownloaded ? '' : ` The first video needs a one-time ${formatBytes(c.modelSizeBytes)} download.`}`
        }</p>
    </div>`;
}

function wireCaptionSettings(container: HTMLElement, ctx: ViewCtx): void {
    const run = async (op: () => Promise<void>) => {
        try {
            await op();
        } catch (err) {
            ctx.showToast(toPlainLanguage(err instanceof Error ? err.message : String(err)));
        }
        ctx.rerender();
    };
    container.querySelector('[data-captions-toggle]')?.addEventListener('click', () => {
        const on = live.captionSettings?.enabled ?? false;
        void run(() => live.setCaptionsEnabled(!on));
    });
    container.querySelectorAll<HTMLButtonElement>('[data-caption-lang]').forEach((el) => {
        el.addEventListener('click', () => void run(() => live.setCaptionLanguage(el.dataset.captionLang as 'en' | 'auto')));
    });
}
