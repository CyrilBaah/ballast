// Home dashboard — ported from the Tether prototype's HomeView.tsx, wired
// to real uploads via ../live.ts. There's no manual pause on the backend
// (only automatic retry-pause and Cancel), so the per-row pause toggle from
// the original mock design is gone.

import { icon, kindIcon, KIND_TONE, statusChip, progressBar, ring, sparkline, avatar, button } from '../components';
import { formatBytes, formatDuration, formatSpeed, relativeTime } from '../format';
import * as live from '../live';
import type { ViewCtx } from '../shell';
import { captionConsentBanner, captionLine } from '../captions';
import { summaryConsentBanner, summaryLine } from '../summaries';
import { wireCaptionConsent } from './transfers';

function greeting(): string {
    const h = new Date().getHours();
    if (h < 5) return 'Still up';
    if (h < 12) return 'Good morning';
    if (h < 18) return 'Good afternoon';
    return 'Good evening';
}

export function renderHome(container: HTMLElement, ctx: ViewCtx): void {
    const uploads = live.orderedUploads();
    const s = live.liveStats();
    const active = uploads.filter((u) => u.status !== 'completed' && u.status !== 'canceled');
    const remaining = s.totalBytes - s.confirmedBytes;
    const eta = s.throughputBps > 0 ? (remaining / s.throughputBps) * 1000 : 0;
    const holding = uploads.filter((u) => u.status === 'reconnecting');
    const overall = s.totalBytes ? s.confirmedBytes / s.totalBytes : 0;
    const first = (live.profile.name || live.profile.email).split(' ')[0];

    const mood = holding.length
        ? {
              tone: 'tangerine' as const,
              ring: '#f97316',
              title: 'The link dropped. Nothing is lost.',
              sub: `${holding.length} transfer${holding.length > 1 ? 's are' : ' is'} holding position. We'll reconnect automatically and continue from the exact byte Drive confirmed.`,
          }
        : active.length
          ? {
                tone: 'grass' as const,
                ring: '#12a970',
                title: 'Everything is moving.',
                sub: `${formatBytes(s.confirmedBytes)} is already safe on Drive. You can close the lid — the journal keeps your place.`,
            }
          : {
                tone: 'sky' as const,
                ring: '#3b82f6',
                title: 'All quiet. Everything delivered.',
                sub: "No transfers in flight. Whenever you're ready, pick a file and we'll take it from there.",
            };

    container.innerHTML = `
        <div class="scroll view-scroll">
            <div class="home-hero mesh-soft grain">
                <div class="home-hero-inner">
                    <div style="display:flex;align-items:center;gap:12px">
                        ${avatar(live.profile.email || live.profile.name, live.profile.name || live.profile.email, 40)}
                        <div>
                            <p style="margin:0;font-size:12.5px;color:var(--color-mute)">${greeting()}, ${first}</p>
                            <h1 class="display" style="margin:0;font-size:26px;line-height:1.2">${mood.title}</h1>
                        </div>
                    </div>
                    <p style="margin-top:12px;max-width:560px;font-size:13.5px;line-height:1.6;color:var(--color-mute)">${mood.sub}</p>

                    <div class="home-hero-grid">
                        <div class="card home-progress-card">
                            <div style="display:flex;align-items:center;gap:16px">
                                ${ring(overall, { size: 72, stroke: 7, color: mood.ring, inner: `<span class="tnum" style="font-size:15px;font-weight:600">${Math.floor(overall * 100)}%</span>` })}
                                <div style="min-width:0;flex:1">
                                    <p class="eyebrow">Everything in your queue</p>
                                    <p class="tnum" style="margin:4px 0 0;font-size:17px;font-weight:500">
                                        ${formatBytes(s.confirmedBytes)}
                                        <span style="font-size:13px;font-weight:400;color:var(--color-faint)">of ${formatBytes(s.totalBytes)} delivered</span>
                                    </p>
                                    <div style="margin-top:10px">${progressBar(overall, 0, { animate: !!active.length && !holding.length, tone: mood.tone === 'tangerine' ? 'tangerine' : 'grass' })}</div>
                                    <p style="margin-top:8px;font-size:12px;color:var(--color-mute)">
                                        ${eta > 0 ? `About ${formatDuration(eta)} to go at ${formatSpeed(s.throughputBps)}` : active.length ? 'Waiting for signal — position saved' : 'Nothing left to send'}
                                    </p>
                                </div>
                            </div>
                        </div>
                        ${statCard('Files delivered', String(s.completedCount), 'confirmed by Google Drive', 'grass', icon.shield())}
                        ${statCard('Active now', String(s.activeCount), 'in your queue', 'grape', icon.refresh())}
                    </div>
                </div>
            </div>

            <div class="home-body">
                <div class="home-main-col">
                    ${live.captionConsent ? captionConsentBanner(live.captionConsent.sizeBytes) : ''}
                    ${live.summaryConsent ? summaryConsentBanner(live.summaryConsent.sizeBytes) : ''}
                    <div class="home-actions">
                        ${actionCard('choose', icon.folder('icon' + ' icon-lg'), 'Choose files', 'Pick a file to send', true)}
                        ${actionCard('lab', icon.flask('icon icon-lg'), 'Test my setup', 'Break it on purpose', false)}
                        ${actionCard('journal', icon.pulse('icon icon-lg'), 'See the proof', 'Every byte, logged', false)}
                    </div>

                    <div style="margin-top:28px;display:flex;align-items:baseline;justify-content:space-between">
                        <h2 class="display" style="margin:0;font-size:19px">In flight</h2>
                        <button type="button" class="link-btn" id="home-view-all">View all transfers</button>
                    </div>

                    <div class="home-transfer-list">
                        ${
                            active.length
                                ? active
                                      .slice(0, 4)
                                      .map((u) => {
                                          const pct = u.sizeBytes > 0 ? u.bytesConfirmed / u.sizeBytes : 0;
                                          const tone = KIND_TONE[u.kind] ?? KIND_TONE.archive;
                                          return `
                                <button type="button" class="card lift home-transfer-row" data-open="${u.id}">
                                    <span class="home-transfer-icon ${tone}">${kindIcon(u.kind, 'icon icon-lg')}</span>
                                    <span class="home-transfer-body">
                                        <span class="home-transfer-top">
                                            <span class="home-transfer-name">${u.name}</span>
                                            ${statusChip(u.status, true)}
                                        </span>
                                        <span style="display:block;margin-top:8px">${progressBar(pct, 0, { height: 6, animate: u.status === 'uploading', tone: u.status === 'reconnecting' || u.status === 'awaiting_confirmation' ? 'tangerine' : 'grass' })}</span>
                                        <span class="home-transfer-meta tnum">
                                            <span style="color:var(--color-ink);font-weight:500">${Math.floor(pct * 100)}%</span>
                                            <span style="color:var(--color-faint)">${formatBytes(u.bytesConfirmed)} / ${formatBytes(u.sizeBytes)}</span>
                                            ${u.status === 'uploading' ? `<span class="fg-grass">${formatSpeed(u.throughputBps)}</span>` : ''}
                                            ${u.status === 'reconnecting' ? `<span class="fg-tangerine">reconnecting…</span>` : ''}
                                            ${u.status === 'awaiting_confirmation' ? `<span class="fg-tangerine">needs your decision</span>` : ''}
                                        </span>
                                        ${captionLine(u.caption, true)}
                                        ${summaryLine(u.summary, true)}
                                    </span>
                                </button>`;
                                      })
                                      .join('')
                                : `<div class="card empty-state">
                                    <span class="empty-state-icon fg-grass" style="background:rgba(18,169,112,0.1)">${icon.check('icon icon-xl')}</span>
                                    <p class="display" style="margin:16px 0 0;font-size:18px">Inbox zero, field edition</p>
                                    <p style="margin:6px auto 0;max-width:340px;font-size:12.5px;line-height:1.6;color:var(--color-mute)">Everything you queued has been confirmed by Google Drive.</p>
                                    <div style="margin-top:20px">${button(`${icon.plus()} Choose files`, { variant: 'primary', attrs: 'id="home-empty-pick"' })}</div>
                                </div>`
                        }
                    </div>
                </div>

                <div class="home-rail">
                    <div class="card rail-card">
                        <div style="display:flex;align-items:center;justify-content:space-between">
                            <h3 class="eyebrow">Your Drive</h3>
                            ${icon.drive('icon fg-faint')}
                        </div>
                        ${
                            live.storageQuota
                                ? `<p class="tnum" style="margin:10px 0 0;font-size:19px;font-weight:500">${formatBytes(live.storageQuota.usageBytes)}</p>
                                   <p style="margin:0;font-size:11.5px;color:var(--color-faint)">used ${live.storageQuota.limitBytes ? `of ${formatBytes(live.storageQuota.limitBytes)}` : '· unlimited storage'}</p>
                                   ${
                                       live.storageQuota.limitBytes
                                           ? `<div style="margin-top:12px">${progressBar(live.storageQuota.usageBytes / live.storageQuota.limitBytes, 0, { height: 7, tone: 'grape' })}</div>`
                                           : ''
                                   }`
                                : `<p class="tnum" style="margin:10px 0 0;font-size:14px;color:var(--color-faint)">Fetching usage…</p>`
                        }
                        <p style="margin-top:10px;font-size:11.5px;line-height:1.5;color:var(--color-mute)">Signed in as <span style="font-weight:500;color:var(--color-ink)">${live.profile.email}</span></p>
                    </div>

                    <div class="card rail-card">
                        <h3 class="eyebrow">Connection today</h3>
                        <p class="tnum" style="margin:8px 0 0;font-size:19px;font-weight:500">${formatSpeed(s.throughputBps)}</p>
                        <div style="margin-top:4px;height:46px">${sparkline(live.netHistory, { color: holding.length ? '#f97316' : '#12a970', height: 46 })}</div>
                        <p style="margin-top:4px;font-size:11.5px;line-height:1.5;color:var(--color-mute)">${holding.length ? "Rough patch — we're waiting it out for you." : 'Steady enough. Every dip is absorbed by chunk retries.'}</p>
                    </div>

                    <div class="card rail-card">
                        <div style="display:flex;align-items:center;justify-content:space-between">
                            <h3 class="eyebrow">Latest activity</h3>
                        </div>
                        <ol class="activity-list">
                            ${
                                live.activityLog.length
                                    ? live.activityLog
                                          .slice(0, 6)
                                          .map(
                                              (e) => `
                                <li class="activity-item">
                                    <span class="activity-dot ${
                                        e.level === 'success' ? 'grass' : e.level === 'warn' ? 'tangerine' : e.level === 'error' ? 'coral' : 'line'
                                    }"></span>
                                    <span>
                                        <span style="display:block;font-size:12px;line-height:1.5;color:var(--color-ink)">${e.message}</span>
                                        <span style="display:block;font-size:10.5px;color:var(--color-faint)">${relativeTime(e.createdAt)}</span>
                                    </span>
                                </li>`,
                                          )
                                          .join('')
                                    : `<li style="font-size:12px;color:var(--color-faint)">Nothing yet — activity shows up here as transfers move.</li>`
                            }
                        </ol>
                    </div>

                    <div class="promise-card">
                        <p style="display:flex;align-items:center;gap:8px;margin:0;font-size:13px;font-weight:500">${icon.heart('icon fg-grass')} A promise, in plain words</p>
                        <p style="margin-top:6px;font-size:12px;line-height:1.6;color:var(--color-mute)">If Ballast says a chunk arrived, it arrived. We write that to disk before we tell you. Worst case after a crash you repeat one chunk — never the file.</p>
                    </div>
                </div>
            </div>
        </div>
    `;

    wireCaptionConsent(container, ctx);
    container.querySelector('#home-view-all')?.addEventListener('click', () => ctx.goto('transfers'));
    container.querySelector('#home-empty-pick')?.addEventListener('click', () => ctx.openPicker());
    container.querySelectorAll<HTMLButtonElement>('[data-action]').forEach((el) => {
        el.addEventListener('click', () => {
            const a = el.dataset.action;
            if (a === 'choose') ctx.openPicker();
            else if (a === 'lab') ctx.goto('lab');
            else if (a === 'journal') ctx.goto('journal');
        });
    });
    container.querySelectorAll<HTMLButtonElement>('[data-open]').forEach((el) => {
        el.addEventListener('click', () => {
            ctx.select(el.dataset.open!);
            ctx.goto('transfers');
        });
    });
}

function statCard(label: string, value: string, sub: string, tone: string, iconSvg: string): string {
    return `<div class="card stat-card">
        <span class="stat-card-icon tone-${tone}">${iconSvg}</span>
        <p class="tnum stat-card-value fg-${tone}">${value}</p>
        <p class="stat-card-label">${label}</p>
        <p class="stat-card-sub">${sub}</p>
    </div>`;
}

function actionCard(action: string, iconSvg: string, title: string, sub: string, primary: boolean): string {
    return `<button type="button" class="lift action-card ${primary ? 'primary' : ''}" data-action="${action}">
        <span class="action-card-icon ${primary ? 'tone-butter' : action === 'lab' ? 'tone-grape' : 'tone-sky'}">${iconSvg}</span>
        <p class="action-card-title">${title}</p>
        <p class="action-card-sub">${sub}</p>
    </button>`;
}
