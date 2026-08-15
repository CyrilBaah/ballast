// Field Lab (network/chaos simulator) — ported from Views.tsx's FieldLab.
// Chaos buttons and network selection only ever touch the local mock store;
// nothing here talks to a real connection.

import { icon } from '../components';
import { formatSpeed } from '../format';
import * as mock from '../mock-data';
import type { ViewCtx } from '../shell';

function bars(n: number, active: boolean): string {
    return `<span class="lab-bars">${[1, 2, 3, 4]
        .map((i) => `<span class="lab-bar ${i <= n ? (active ? 'active' : 'mute') : ''}" style="height:${5 + i * 3.5}px"></span>`)
        .join('')}</span>`;
}

export function renderLab(container: HTMLElement, ctx: ViewCtx): void {
    const s = mock.settings;

    container.innerHTML = `
        <div class="scroll view-scroll lab-scroll">
            <div class="lab-inner">
                <header>
                    <span class="pill-badge tone-grape">${icon.flask('icon')} Safe to break things</span>
                    <h1 class="display" style="margin:12px 0 0;font-size:27px;line-height:1.2">Rehearse the worst day you'll ever have</h1>
                    <p style="margin:8px 0 0;max-width:600px;font-size:13.5px;line-height:1.6;color:var(--color-mute)">Cut the signal, kill the app, reboot the laptop — then watch Ballast come back at the exact byte. Better to find out here than on a deadline.</p>
                </header>

                <section class="lab-grid-top">
                    <div class="card" style="padding:20px">
                        <h2 class="eyebrow">How's the signal?</h2>
                        <div class="lab-link-list">
                            ${mock.LINKS.map((l) => {
                                const active = s.networkQuality === l.id;
                                return `
                                <button type="button" class="lab-link-row ${active ? 'active' : ''}" data-link="${l.id}">
                                    <span style="font-size:17px">${l.emoji}</span>
                                    ${bars(l.bars, active)}
                                    <span style="min-width:0;flex:1">
                                        <span style="display:block;font-size:13px;font-weight:500;color:var(--color-ink)">${l.label}</span>
                                        <span style="display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:11.5px;color:var(--color-faint)">${l.sub}</span>
                                    </span>
                                    <span style="text-align:right">
                                        <span class="tnum" style="display:block;font-size:12px;font-weight:500;color:var(--color-ink)">${l.bps ? formatSpeed(l.bps) : '—'}</span>
                                        <span class="tnum" style="display:block;font-size:10.5px;color:var(--color-faint)">drops ${l.loss}</span>
                                    </span>
                                </button>`;
                            }).join('')}
                        </div>
                    </div>

                    <div style="display:flex;flex-direction:column;gap:16px">
                        <div class="card" style="padding:20px">
                            <h2 class="eyebrow">Chaos buttons</h2>
                            <div class="lab-chaos-list">
                                ${chaosBtn('drop', icon.alert('icon icon-lg'), 'Yank the connection', 'Sockets die mid-chunk', 'tangerine')}
                                ${chaosBtn('crash', icon.bolt('icon icon-lg'), 'Kill the app', 'No graceful shutdown', 'coral')}
                                ${chaosBtn('reboot', icon.refresh('icon icon-lg'), 'Reboot the laptop', 'Cold start, journal replay', 'grape')}
                            </div>
                        </div>

                        <div class="card mesh-photo lab-photo-card">
                            <div class="lab-photo-caption">
                                <p style="margin:0;font-size:13px;font-weight:500;color:#fff">Built for porches and pickups</p>
                                <p style="margin:2px 0 0;font-size:11.5px;line-height:1.5;color:rgba(255,255,255,0.8)">Every acknowledged byte hits the local journal before the interface moves.</p>
                            </div>
                        </div>
                    </div>
                </section>

                <section class="lab-sliders">
                    ${slider('bandwidth', 'Bandwidth cap', s.bandwidthCapMbps ? `${s.bandwidthCapMbps} Mbps` : 'Unlimited', 'Leave headroom so you can still text home.', 0, 500, 10, s.bandwidthCapMbps)}
                    ${slider('concurrent', 'Parallel transfers', `${s.maxConcurrent} at a time`, 'Fewer streams survive flaky radios better.', 1, 5, 1, s.maxConcurrent)}
                    ${slider('timescale', 'Demo clock', `${(s.timeScale / 100).toFixed(0)}× real time`, 'Compress hours of field work into minutes.', 100, 12000, 100, s.timeScale)}
                </section>
            </div>
        </div>
    `;

    container.querySelectorAll<HTMLButtonElement>('[data-link]').forEach((el) => {
        el.addEventListener('click', () => {
            mock.settings.networkQuality = el.dataset.link as mock.NetworkQuality;
            ctx.rerender();
        });
    });
    container.querySelectorAll<HTMLButtonElement>('[data-chaos]').forEach((el) => {
        el.addEventListener('click', () => {
            const a = el.dataset.chaos;
            if (a === 'drop') ctx.showToast('Link cut — positions held at last confirmed byte.');
            if (a === 'crash') ctx.showToast('App killed — journal replayed, nothing lost.');
            if (a === 'reboot') ctx.showToast('Rebooted — every session picked back up.');
        });
    });

    wireSlider(container, 'bandwidth', (v) => {
        mock.settings.bandwidthCapMbps = v;
        return v ? `${v} Mbps` : 'Unlimited';
    });
    wireSlider(container, 'concurrent', (v) => {
        mock.settings.maxConcurrent = v;
        return `${v} at a time`;
    });
    wireSlider(container, 'timescale', (v) => {
        mock.settings.timeScale = v;
        return `${(v / 100).toFixed(0)}× real time`;
    });
}

function wireSlider(container: HTMLElement, key: string, apply: (v: number) => string) {
    const input = container.querySelector<HTMLInputElement>(`#slider-${key}`);
    const label = container.querySelector<HTMLElement>(`#slider-${key}-value`);
    input?.addEventListener('input', () => {
        if (label) label.textContent = apply(Number(input.value));
    });
}

function chaosBtn(action: string, iconSvg: string, title: string, sub: string, tone: string): string {
    return `<button type="button" class="lab-chaos-btn tone-${tone}" data-chaos="${action}">
        ${iconSvg}
        <span style="min-width:0;flex:1">
            <span style="display:block;font-size:13px;font-weight:500;color:var(--color-ink)">${title}</span>
            <span style="display:block;font-size:11.5px;color:var(--color-mute)">${sub}</span>
        </span>
        ${icon.chevron('icon')}
    </button>`;
}

function slider(key: string, label: string, value: string, hint: string, min: number, max: number, step: number, v: number): string {
    return `<div class="card" style="padding:20px">
        <h2 class="eyebrow">${label}</h2>
        <p class="tnum" id="slider-${key}-value" style="margin:8px 0 0;font-size:19px;font-weight:500">${value}</p>
        <input type="range" id="slider-${key}" min="${min}" max="${max}" step="${step}" value="${v}" style="margin-top:14px" />
        <p style="margin-top:10px;font-size:11.5px;line-height:1.5;color:var(--color-mute)">${hint}</p>
    </div>`;
}
